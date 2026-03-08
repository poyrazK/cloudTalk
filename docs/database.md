# Database

## Engine

PostgreSQL 16 via `pgx/v5` connection pool.

## Schema

### `users`
| Column        | Type        | Notes              |
|---------------|-------------|--------------------|
| id            | UUID PK     | gen_random_uuid()  |
| username      | TEXT UNIQUE |                    |
| email         | TEXT UNIQUE |                    |
| password_hash | TEXT        | bcrypt             |
| created_at    | TIMESTAMPTZ |                    |

### `rooms`
| Column      | Type        | Notes              |
|-------------|-------------|--------------------|
| id          | UUID PK     |                    |
| name        | TEXT        |                    |
| description | TEXT        |                    |
| created_by  | UUID FK     | → users(id)        |
| created_at  | TIMESTAMPTZ |                    |

### `room_members`
| Column    | Type        | Notes              |
|-----------|-------------|--------------------|
| room_id   | UUID FK     | → rooms(id)        |
| user_id   | UUID FK     | → users(id)        |
| joined_at | TIMESTAMPTZ |                    |

PK is `(room_id, user_id)`.

### `messages`
| Column     | Type        | Notes              |
|------------|-------------|--------------------|
| id         | UUID PK     |                    |
| room_id    | UUID FK     | → rooms(id)        |
| sender_id  | UUID FK     | → users(id)        |
| content    | TEXT        |                    |
| created_at | TIMESTAMPTZ | indexed DESC       |

Index: `(room_id, created_at DESC)` for efficient cursor pagination.

### `direct_messages`
| Column      | Type        | Notes              |
|-------------|-------------|--------------------|
| id          | UUID PK     |                    |
| sender_id   | UUID FK     | → users(id)        |
| receiver_id | UUID FK     | → users(id)        |
| content     | TEXT        |                    |
| created_at  | TIMESTAMPTZ |                    |

Index on `(LEAST(sender, receiver), GREATEST(sender, receiver), created_at DESC)` for bidirectional DM queries.

### `refresh_tokens`
| Column     | Type        | Notes              |
|------------|-------------|--------------------|
| id         | UUID PK     |                    |
| user_id    | UUID FK     | → users(id)        |
| token_hash | TEXT        | SHA-256 of raw token |
| expires_at | TIMESTAMPTZ |                    |

## Migrations

Managed with [golang-migrate](https://github.com/golang-migrate/migrate). Migration files live in `migrations/`.

Files follow the naming convention: `NNN_description.up.sql` / `NNN_description.down.sql`.

Migrations run automatically on server startup via `db.Migrate()`.

**Manual run:**
```bash
migrate -path migrations -database "$DATABASE_DSN" up
migrate -path migrations -database "$DATABASE_DSN" down 1
```

## Pagination

All message history endpoints use **cursor-based pagination** (`created_at < :before`). This avoids the performance degradation of SQL `OFFSET` on large tables.

Example: to fetch the next page, pass the `created_at` of the oldest message from the previous page as the `before` query parameter.
