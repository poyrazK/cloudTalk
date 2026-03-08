# Auth

## Tokens

cloudTalk uses a two-token scheme:

| Token         | Type    | TTL     | Storage                        |
|---------------|---------|---------|--------------------------------|
| Access token  | JWT HS256 | 15 min | Client-side only               |
| Refresh token | Opaque  | 7 days  | SHA-256 hash stored in DB      |

## Access Token

Signed with HMAC-SHA256 using the `JWT_SECRET` env var. Contains only the user's UUID as the `sub` claim. No user data or roles are embedded.

**Usage:** `Authorization: Bearer <access_token>` on all protected REST endpoints.

For WebSocket: `GET /ws?token=<access_token>`

## Refresh Token

A cryptographically-random 32-byte value (hex-encoded, 64 chars). Only its SHA-256 hash is persisted — the raw value is never stored.

**Flow:**
1. Client calls `POST /auth/login` → receives both tokens.
2. When the access token expires, call `POST /auth/refresh` with the refresh token → new access token.
3. Call `POST /auth/logout` to revoke the refresh token immediately.

## Middleware

`internal/auth/middleware.go` — validates the Bearer JWT and injects the `uuid.UUID` user ID into the request context. Downstream handlers retrieve it with `auth.UserIDFromContext(ctx)`.
