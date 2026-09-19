# Syntroform Backend

High-performance Go backend service for Syntroform.

## Features
- PostgreSQL persistence with SQL migrations
- JWT authentication & session management
- Form definitions & response ingestion
- Drop-off / Partial submission tracking
- Adaptive AI follow-up reasoning (Groq LLM)
- Real-time response analytics

## Setup & Running

1. Copy environment variables:
```bash
cp .env.example .env
```

2. Run database migrations and start the server:
```bash
go run cmd/server/main.go
```

## Docker

```bash
docker build -t syntroform-backend .
docker run -p 8080:8080 --env-file .env syntroform-backend
```
