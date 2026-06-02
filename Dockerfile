
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o main ./cmd

FROM alpine:latest
WORKDIR /app

COPY --from=builder /app/main .
COPY frontend ./frontend

RUN apk add --no-cache postgresql-client

EXPOSE 8080

CMD ["./main"]