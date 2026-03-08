# Configuration

All configuration is provided via environment variables. Copy `.env.example` to `.env` for local development.

| Variable           | Required | Default                                                                  | Description                                      |
|--------------------|----------|--------------------------------------------------------------------------|--------------------------------------------------|
| `PORT`             | No       | `8080`                                                                   | HTTP server port                                 |
| `DATABASE_DSN`     | Yes      | `postgres://postgres:postgres@localhost:5432/cloudtalk?sslmode=disable`  | PostgreSQL connection string                     |
| `KAFKA_BROKERS`    | Yes      | `localhost:9092`                                                         | Comma-separated Kafka broker addresses           |
| `KAFKA_GROUP_ID`   | No       | `cloudtalk-<hostname>`                                                   | Kafka consumer group ID (must be unique per pod) |
| `JWT_SECRET`       | Yes      | `change-me-in-production`                                                | HMAC secret for signing JWT access tokens        |
| `JWT_EXP_MINUTES`  | No       | `15`                                                                     | Access token TTL in minutes                      |
| `REFRESH_EXP_DAYS` | No       | `7`                                                                      | Refresh token TTL in days                        |

> **Production note:** Always override `JWT_SECRET` with a strong random value (e.g. `openssl rand -hex 32`). Never use the default.
