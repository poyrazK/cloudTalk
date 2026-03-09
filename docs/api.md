# API Reference

Base path: `/api/v1`

Protected routes require: `Authorization: Bearer <access_token>`

---

## Auth

### POST /auth/register
Create a new account.

**Request**
```json
{ "username": "alice", "email": "alice@example.com", "password": "secret" }
```
**Response** `201`
```json
{ "id": "<uuid>", "username": "alice", "email": "alice@example.com", "created_at": "..." }
```

---

### POST /auth/login
**Request**
```json
{ "email": "alice@example.com", "password": "secret" }
```
**Response** `200`
```json
{ "access_token": "<jwt>", "refresh_token": "<opaque>" }
```

---

### POST /auth/refresh
**Request**
```json
{ "refresh_token": "<opaque>" }
```
**Response** `200`
```json
{ "access_token": "<jwt>" }
```

---

### POST /auth/logout
**Request**
```json
{ "refresh_token": "<opaque>" }
```
**Response** `204`

---

## Rooms

### POST /rooms
Create a room. Creator is automatically added as a member.

**Request**
```json
{ "name": "general", "description": "General discussion" }
```
**Response** `201`
```json
{ "id": "<uuid>", "name": "general", "description": "...", "created_by": "<uuid>", "created_at": "..." }
```

---

### GET /rooms
List all rooms the authenticated user is a member of.

**Response** `200` — array of room objects.

---

### GET /rooms/:id
Get a single room by ID.

**Response** `200` — room object, `404` if not found.

---

### POST /rooms/:id/join
Join a room.

**Response** `204`

---

### POST /rooms/:id/leave
Leave a room.

**Response** `204`

---

### GET /rooms/:id/messages
Paginated message history, newest first.

Only room members can access this endpoint. Non-members receive `403`.

**Query params**

| Param  | Type          | Default  | Description                          |
|--------|---------------|----------|--------------------------------------|
| before | RFC3339Nano   | now      | Return messages older than this time |
| limit  | int (1–100)   | 50       | Number of messages to return         |

**Response** `200` — array of message objects.
```json
[
  {
    "id": "<uuid>",
    "room_id": "<uuid>",
    "sender_id": "<uuid>",
    "content": "Hello",
    "created_at": "...",
    "edited_at": "...",
    "deleted_at": "..."
  }
]
```

---

## Direct Messages

### GET /dms/:userId/messages
Paginated DM history between the authenticated user and `:userId`, newest first.

Requests where `:userId` equals the authenticated user are rejected with `400`.

**Query params** — same as room messages (`before`, `limit`).

**Response** `200` — array of direct message objects.
```json
[
  {
    "id": "<uuid>",
    "sender_id": "<uuid>",
    "receiver_id": "<uuid>",
    "content": "Hey",
    "created_at": "...",
    "delivered_at": "...",
    "read_at": "...",
    "edited_at": "...",
    "deleted_at": "..."
  }
]
```

- `delivered_at` is set when the recipient WebSocket connection receives the DM event.
- `read_at` is set when the recipient marks the DM as read over WebSocket.
- `edited_at` is set when the sender edits the message.
- `deleted_at` is set when the sender soft deletes the message.

### GET /dms/unread-counts
Returns unread inbound DM counts grouped by sender user.

**Response** `200`
```json
[
  { "user_id": "<uuid>", "count": 3 },
  { "user_id": "<uuid>", "count": 1 }
]
```

Only messages where the authenticated user is the `receiver_id` and `read_at` is null are counted.

---

## Misc

### GET /health
Returns `200 ok`. Used by Kubernetes probes.

### GET /ws
WebSocket upgrade endpoint. See [WebSocket Protocol](websocket.md).
