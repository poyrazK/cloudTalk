# Configuration

All configuration is provided via environment variables. Copy `.env.example` to `.env` for local development.

| Variable           | Required | Default                                                                  | Description                                      |
|--------------------|----------|--------------------------------------------------------------------------|--------------------------------------------------|
| `APP_ENV`          | No       | `dev`                                                                    | Runtime mode: `dev` or `prod`                    |
| `PORT`             | No       | `8080`                                                                   | HTTP server port                                 |
| `DATABASE_DSN`     | Yes      | `postgres://postgres:postgres@localhost:5432/cloudtalk?sslmode=disable`  | PostgreSQL connection string                     |
| `KAFKA_BROKERS`    | Yes      | `localhost:9092`                                                         | Comma-separated Kafka broker addresses           |
| `KAFKA_GROUP_ID`   | No       | `cloudtalk-<hostname>`                                                   | Kafka consumer group ID (must be unique per pod) |
| `JWT_SECRET`       | Yes      | `change-me-in-production`                                                | HMAC secret for signing JWT access tokens        |
| `JWT_EXP_MINUTES`  | No       | `15`                                                                     | Access token TTL in minutes                      |
| `REFRESH_EXP_DAYS` | No       | `7`                                                                      | Refresh token TTL in days                        |
| `ALLOWED_ORIGINS`  | No       | empty                                                                    | Comma-separated allowed browser origins          |
| `AUTH_RATE_LIMIT_RPM` | No    | `20`                                                                     | Auth route rate limit per minute                 |

## Validation Rules

Startup fails fast on invalid configuration.

- `APP_ENV` must be `dev` or `prod`
- numeric env vars must be valid positive integers within sane ranges
- `DB_MIN_CONNS` must not exceed `DB_MAX_CONNS`
- each `ALLOWED_ORIGINS` entry must be a valid absolute origin URL

## Production Requirements

When `APP_ENV=prod`, startup fails if:

- `JWT_SECRET` is the default placeholder or shorter than 32 characters
- `ALLOWED_ORIGINS` is empty
- `DATABASE_DSN` points to localhost
- `KAFKA_BROKERS` is localhost-only

Use a strong JWT secret, explicit browser origins, and non-local infrastructure endpoints before deploying.
