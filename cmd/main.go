package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ══════════════════════════════════════════════════════════════════════════════
//  MODELS
// ══════════════════════════════════════════════════════════════════════════════

// User is the main account model.
// RecoveryEmail is stored AES-256-GCM encrypted in the database.
// PasswordHash is excluded from every JSON response via `json:"-"`.
type User struct {
	ID            uint           `json:"id"       gorm:"primarykey;autoIncrement"`
	CreatedAt     time.Time      `json:"-"`
	UpdatedAt     time.Time      `json:"-"`
	DeletedAt     gorm.DeletedAt `json:"-"        gorm:"index"`
	Username      string         `json:"username" gorm:"uniqueIndex;not null"`
	PasswordHash  string         `json:"-"        gorm:"not null"`
	Gold          int            `json:"gold"     gorm:"default:0"`
	RecoveryEmail string         `json:"-"` // encrypted before write, decrypted on read
}

// Cat mirrors the shape the frontend already expects (see script.js createCatCard).
type Cat struct {
	ID         uint           `json:"id"           gorm:"primarykey;autoIncrement"`
	CreatedAt  time.Time      `json:"-"`
	UpdatedAt  time.Time      `json:"-"`
	DeletedAt  gorm.DeletedAt `json:"-"            gorm:"index"`
	UserID     uint           `json:"user_id"      gorm:"not null;index"`
	Name       string         `json:"name"         gorm:"not null"`
	Rarity     string         `json:"rarity"       gorm:"not null;default:'COMMON'"`
	GPM        int            `json:"gpm"          gorm:"column:gpm;not null;default:10"`
	Type       string         `json:"type"`
	ImageURL   string         `json:"image_url"`
	IsOnMarket bool           `json:"is_on_market" gorm:"default:false"`
}

// ══════════════════════════════════════════════════════════════════════════════
//  GLOBALS
// ══════════════════════════════════════════════════════════════════════════════

var (
	db        *gorm.DB
	jwtSecret []byte
	aesKey    []byte
)

// getenv returns the environment variable or a fallback value.
func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ══════════════════════════════════════════════════════════════════════════════
//  AES-256-GCM  —  symmetric encryption for sensitive model fields
// ══════════════════════════════════════════════════════════════════════════════

// initAESKey reads the 32-byte key from AES_KEY or uses a static dev default.
// In production, set AES_KEY to a securely-generated 32-byte random string.
func initAESKey() {
	raw := getenv("AES_KEY", "01234567890123456789012345678901")
	if len(raw) != 32 {
		log.Fatal("[FATAL] AES_KEY must be exactly 32 bytes for AES-256. Got: ", len(raw))
	}
	aesKey = []byte(raw)
}

// encrypt encodes plaintext with AES-256-GCM.
// Output format: base64( nonce || ciphertext || auth-tag )
func encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	// gcm.Seal prepends the nonce so decrypt can extract it later.
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// decrypt reverses encrypt and returns the original plaintext.
func decrypt(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return "", errors.New("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// ══════════════════════════════════════════════════════════════════════════════
//  JWT
// ══════════════════════════════════════════════════════════════════════════════

// Claims embeds the standard JWT fields plus our user-specific payload.
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// generateJWT creates a signed HS256 token valid for 24 hours.
func generateJWT(u User) (string, error) {
	claims := Claims{
		UserID:   u.ID,
		Username: u.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
}

// parseJWT validates and parses the token string, returning the claims.
func parseJWT(raw string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(raw, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	if c, ok := tok.Claims.(*Claims); ok && tok.Valid {
		return c, nil
	}
	return nil, errors.New("invalid token")
}

// ══════════════════════════════════════════════════════════════════════════════
//  MIDDLEWARE
// ══════════════════════════════════════════════════════════════════════════════

// authMiddleware validates the Bearer token and injects userID / username into
// the gin.Context so handlers do not have to parse the token themselves.
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				gin.H{"message": "authorization header missing or malformed"})
			return
		}
		claims, err := parseJWT(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				gin.H{"message": "invalid or expired token"})
			return
		}
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}

// ══════════════════════════════════════════════════════════════════════════════
//  HANDLERS — AUTH
// ══════════════════════════════════════════════════════════════════════════════

// POST /api/v1/register
// Hashes the password with bcrypt, encrypts the recovery email with AES,
// persists the user, seeds starter cats, and returns a JWT.
func registerHandler(c *gin.Context) {
	var req struct {
		Username      string `json:"username"       binding:"required,min=3,max=32"`
		Password      string `json:"password"       binding:"required,min=6"`
		RecoveryEmail string `json:"recovery_email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	// --- bcrypt hash ---
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to hash password"})
		return
	}

	// --- AES-256-GCM encrypt the recovery email ---
	var encEmail string
	if req.RecoveryEmail != "" {
		if encEmail, err = encrypt(req.RecoveryEmail); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "encryption error"})
			return
		}
	}

	user := User{
		Username:      req.Username,
		PasswordHash:  string(hash),
		Gold:          100, // welcome bonus
		RecoveryEmail: encEmail,
	}

	if res := db.Create(&user); res.Error != nil {
		if isDuplicateKey(res.Error) {
			c.JSON(http.StatusConflict, gin.H{"message": "username already taken"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": res.Error.Error()})
		return
	}

	seedStarterCats(user.ID) // give new players 3 starter cats

	token, err := generateJWT(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "could not generate token"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"token": token,
		"user":  gin.H{"id": user.ID, "username": user.Username, "gold": user.Gold},
	})
}

// POST /api/v1/login
// Verifies credentials with bcrypt.CompareHashAndPassword and returns a JWT.
func loginHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	var user User
	if err := db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		// Generic message — do not reveal whether the username exists.
		c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid credentials"})
		return
	}

	token, err := generateJWT(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "could not generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  gin.H{"id": user.ID, "username": user.Username, "gold": user.Gold},
	})
}

// GET /api/v1/me  (auth required)
// Returns the current user's profile, decrypting RecoveryEmail on the fly.
func meHandler(c *gin.Context) {
	uid, _ := c.Get("userID")
	var user User
	if err := db.First(&user, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "user not found"})
		return
	}

	decEmail := ""
	if user.RecoveryEmail != "" {
		var err error
		if decEmail, err = decrypt(user.RecoveryEmail); err != nil {
			// Log but never expose raw decryption errors to the client.
			log.Printf("[WARN] decrypt error for user %d: %v", user.ID, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":             user.ID,
		"username":       user.Username,
		"gold":           user.Gold,
		"recovery_email": decEmail,
	})
}

// ══════════════════════════════════════════════════════════════════════════════
//  HANDLERS — GAME (cats)
// ══════════════════════════════════════════════════════════════════════════════

// GET /api/v1/cats  (auth required)
// Validates the JWT via authMiddleware, then fetches only cats owned by the
// authenticated user and returns them as JSON.
func catsHandler(c *gin.Context) {
	uid, _ := c.Get("userID") // injected by authMiddleware
	var cats []Cat
	if err := db.Where("user_id = ?", uid).Find(&cats).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cats": cats})
}

// ══════════════════════════════════════════════════════════════════════════════
//  HANDLERS — ADMIN (backup / restore)
// ══════════════════════════════════════════════════════════════════════════════

// POST /api/v1/admin/backup
// Calls pg_dump to create a timestamped .sql dump in /tmp.
// In production, consider streaming the dump to object storage (S3, GCS, etc.)
// and adding an admin-role check on top of the JWT check.
func backupHandler(c *gin.Context) {
	outFile := "/tmp/backup_" + time.Now().Format("20060102_150405") + ".sql"

	cmd := buildPGCommand("pg_dump", []string{"-f", outFile})
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[ERROR] pg_dump: %s", out)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "backup failed",
			"detail":  string(out),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "backup created successfully",
		"file":    outFile,
	})
}

// POST /api/v1/admin/restore   body: { "file": "/tmp/backup_YYYYMMDD_HHMMSS.sql" }
// Restores the database from a previously created pg_dump file.
// The file path is strictly validated against our own naming convention.
func restoreHandler(c *gin.Context) {
	var req struct {
		File string `json:"file" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	// Path allow-list: only files we created with backupHandler are accepted.
	// This prevents directory-traversal and arbitrary file execution.
	if !strings.HasPrefix(req.File, "/tmp/backup_") || !strings.HasSuffix(req.File, ".sql") {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid backup file path"})
		return
	}

	cmd := buildPGCommand("psql", []string{"-f", req.File})
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[ERROR] psql restore: %s", out)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "restore failed",
			"detail":  string(out),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "database restored successfully"})
}

// ══════════════════════════════════════════════════════════════════════════════
//  HELPERS
// ══════════════════════════════════════════════════════════════════════════════

// buildPGCommand constructs a pg_dump / psql exec.Cmd from environment variables.
// Supports both a full DATABASE_URL connection string and individual PG* vars.
func buildPGCommand(binary string, extraArgs []string) *exec.Cmd {
	var args []string
	env := os.Environ()

	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		// Connection URI form: pg_dump "postgresql://user:pass@host/db" -f out.sql
		args = append([]string{dsn}, extraArgs...)
	} else {
		// Individual host/user/db flags form
		args = append([]string{
			"-h", getenv("PGHOST", "localhost"),
			"-p", getenv("PGPORT", "5432"),
			"-U", getenv("PGUSER", "postgres"),
			"-d", getenv("PGDATABASE", "postgres"),
		}, extraArgs...)
		// Inject password via env so it never appears in the process list
		env = append(env, "PGPASSWORD="+getenv("PGPASSWORD", "postgres"))
	}

	cmd := exec.Command(binary, args...)
	cmd.Env = env
	return cmd
}

// isDuplicateKey detects PostgreSQL unique-constraint violations.
func isDuplicateKey(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "unique") ||
		strings.Contains(msg, "23505") // PostgreSQL error code for unique_violation
}

// seedStarterCats inserts three starter cats for a brand-new user so the
// frontend renders content immediately after the first login.
func seedStarterCats(userID uint) {
	const placeholder = "https://png.pngtree.com/png-clipart/20250807/original/" +
		"pngtree-cute-chibi-cat-illustration-pastel-colors-minimalist-flat-design-png-image_21644944.png"

	starters := []Cat{
		{UserID: userID, Name: "Fluffy", Rarity: "COMMON", GPM: 10, Type: "Fluffy", ImageURL: placeholder},
		{UserID: userID, Name: "Shadow", Rarity: "RARE", GPM: 50, Type: "Sphynx", ImageURL: placeholder, IsOnMarket: true},
		{UserID: userID, Name: "Higl", Rarity: "LEGENDARY", GPM: 250, Type: "Siamese", ImageURL: placeholder},
	}
	if res := db.Create(&starters); res.Error != nil {
		log.Printf("[WARN] seedStarterCats: %v", res.Error)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
//  MAIN
// ══════════════════════════════════════════════════════════════════════════════

func main() {
	// --- Init crypto keys ---
	initAESKey()
	jwtSecret = []byte(getenv("JWT_SECRET", "change-me-in-production-please"))

	// --- Database ---
	dsn := getenv("DATABASE_URL",
		"host=localhost user=postgres password=postgres dbname=postgres port=5432 sslmode=disable")

	var err error
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("[FATAL] DB connection error: ", err)
	}

	if err = db.AutoMigrate(&User{}, &Cat{}); err != nil {
		log.Fatal("[FATAL] AutoMigrate error: ", err)
	}

	// --- Router ---
	r := gin.Default()
	r.Static("/static", "./frontend")
    r.StaticFile("/", "./frontend/index.html")
    r.StaticFile("/login.html", "./frontend/login.html")
    r.StaticFile("/styles.css", "./frontend/styles.css")
    r.StaticFile("/script.js", "./frontend/script.js")

	// CORS — tighten AllowOrigins to your domain before going to production
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// Health check (unauthenticated)
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong", "status": "DB working!"})
	})

	v1 := r.Group("/api/v1")
	{
		// ── Public ──────────────────────────────────────────────────────────
		v1.POST("/register", registerHandler)
		v1.POST("/login", loginHandler)

		// ── Auth-protected ───────────────────────────────────────────────────
		protected := v1.Group("", authMiddleware())
		{
			protected.GET("/me", meHandler)
			protected.GET("/cats", catsHandler)
		}

		// ── Admin ── TODO: add role check before shipping to production ──────
		admin := v1.Group("/admin", authMiddleware())
		{
			admin.POST("/backup", backupHandler)
			admin.POST("/restore", restoreHandler)
		}
	}

	log.Println("[INFO] Server starting on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}