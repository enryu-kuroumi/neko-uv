# NekoUV 

NekoUV is a lightweight, fully containerized web application built to practice secure backend development, system interactions principles. The project features a REST API that handles a cat collection mechanic alongside secure user authentication.

 **deployment automation and environment isolation** using Docker and Docker Compose.

---

##  Tech Stack & Infrastructure

* **Backend:** Go (Gin Web Framework, GORM)
* **Database:** PostgreSQL
* **Frontend:** Vanilla JS, HTML5, CSS3 (Tailwind CSS)
* **DevOps & Security:** Docker, Docker Compose, JWT, bcrypt, AES-256-GCM

---

##  Key Infrastructure & DevOps Features

* **Zero-Configuration Deployment:** The entire application stack (Go API, PostgreSQL database, and frontend static routing) is orchestrated via Docker Compose.
* **12-Factor App Principles:** Sensitive configuration data (such as `JWT_SECRET`, `DATABASE_URL`, and cryptographic keys) are injected entirely via environment variables, keeping the codebase clean and production-ready.
* **Data Persistence:** Integrated Docker Volumes to ensure PostgreSQL data survives container restarts and upgrades.
* **Crypto Security at Rest:** Passwords are securely hashed using `bcrypt`. Additionally, sensitive user details (like recovery emails) are symmetrically encrypted using **AES-256-GCM** before being written to the database.

---
##  How to Run 

Make sure you have Docker and Docker Compose installed on your system (or running inside WSL/VirtualBox).

1. Clone the repository:

git clone https://github.com/enryu-kuroumi/neko-uv.git


2. Navigate into the folder:

cd neko-uv

3. Start the entire ecosystem with a single command:

docker-compose up --build -d


4. Open your browser and navigate to:

http://localhost:8080
