# Configuration

All configuration is provided via environment variables. Copy `.env.example` to `.env` for local development.

| Variable           | Required | Default                                                                  | Description                                      |
|--------------------|----------|--------------------------------------------------------------------------|--------------------------------------------------|
| `APP_ENV`          | No       | `dev`                                                                    | Runtime mode: `dev` or `prod`                    |
| `TRACING_ENABLED`  | No       | `false`                                                                  | Enables OpenTelemetry tracing                    |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | No | `http://localhost:4318/v1/traces`                                 | OTLP traces endpoint                             |
| `OTEL_TRACES_SAMPLER_ARG` | No | `0.1`                                                              | Trace sampling ratio (`0.0`-`1.0`)               |
| `OTEL_SERVICE_NAME` | No      | `cloudtalk`                                                              | Traced service name                              |
| `OTEL_SERVICE_VERSION` | No   | `dev`                                                                    | Traced service version label                     |
| `WS_CHAT_RPS`      | No       | `5`                                                                      | WebSocket chat send events per second            |
| `WS_CHAT_BURST`    | No       | `10`                                                                     | WebSocket chat send burst size                   |
| `WS_TYPING_RPS`    | No       | `2`                                                                      | WebSocket typing events per second               |
| `WS_TYPING_BURST`  | No       | `4`                                                                      | WebSocket typing burst size                      |
| `WS_READ_RPS`      | No       | `10`                                                                     | WebSocket read events per second                 |
| `WS_READ_BURST`    | No       | `20`                                                                     | WebSocket read burst size                        |
| `WS_ROOM_RPS`      | No       | `5`                                                                      | WebSocket room join/leave events per second      |
| `WS_ROOM_BURST`    | No       | `10`                                                                     | WebSocket room join/leave burst size             |
| `HTTP_CONVERSATION_READ_RPM` | No | `120`                                                             | Per-user HTTP conversation and unread read limit |
| `HTTP_MESSAGE_HISTORY_RPM` | No | `180`                                                               | Per-user HTTP message history read limit         |
| `HTTP_ROOM_ACTION_RPM` | No    | `30`                                                                | Per-user HTTP room create/join/leave limit       |
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
- WebSocket throttle RPS values must be valid positive floats; burst values (for example `WS_*_BURST`) must be valid positive integers
- HTTP route-group rate limits must be valid positive integers
- `OTEL_TRACES_SAMPLER_ARG` must be a valid float between `0` and `1`
- `DB_MIN_CONNS` must not exceed `DB_MAX_CONNS`
- each `ALLOWED_ORIGINS` entry must be a valid absolute origin URL
- when tracing is enabled, `OTEL_EXPORTER_OTLP_ENDPOINT` must be a valid URL and `OTEL_SERVICE_NAME` must be set

## Production Requirements

When `APP_ENV=prod`, startup fails if:

- `JWT_SECRET` is the default placeholder or shorter than 32 characters
- `ALLOWED_ORIGINS` is empty
- `DATABASE_DSN` points to localhost
- `KAFKA_BROKERS` is localhost-only

Use a strong JWT secret, explicit browser origins, and non-local infrastructure endpoints before deploying.
