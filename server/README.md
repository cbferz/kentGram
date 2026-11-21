# kentGram API

Initial lightweight Go API for the Android client.

## Included
- PostgreSQL connection pool and initial schema
- Health check: `GET /health`
- Registration: `POST /api/v1/auth/register`
- Login: `POST /api/v1/auth/login`
- Current user: `GET /api/v1/me` with a Bearer access token
- Password hashing with bcrypt and signed JWT access tokens

## VPS deployment

Requirements: Ubuntu 22.04/24.04 or Debian 12/13, Docker Engine and Docker Compose plugin.

1. Clone the repository.
2. Copy `.env.example` to `.env`.
3. Replace both secrets with long random values. Never commit `.env`.
4. Run `docker compose up -d --build`.
5. Check `curl http://127.0.0.1:8080/health`.

The API binds to loopback only. Put a TLS reverse proxy such as Caddy or Nginx in front of it before exposing it publicly.

## Resource profile
The compose setup intentionally starts only the API and PostgreSQL. Redis, media processing and separate workers are deferred to avoid wasting the 4 GB VPS memory budget.
