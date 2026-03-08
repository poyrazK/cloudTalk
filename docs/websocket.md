# WebSocket Protocol

## Connection

```
GET /ws?token=<access_token>
```

Authentication is via the JWT access token as a query parameter. The connection is rejected with `401` if the token is missing or invalid.

After a successful upgrade, the client must send `join_room` for each room it wants to receive messages from.

---

## Client → Server Messages

All messages are JSON text frames.

### join_room
Subscribe to messages in a room.
```json
{ "type": "join_room", "room_id": "<uuid>" }
```

### leave_room
Unsubscribe from a room.
```json
{ "type": "leave_room", "room_id": "<uuid>" }
```

### message
Send a message to a room. The sender must be a room member.
```json
{ "type": "message", "room_id": "<uuid>", "content": "Hello everyone!" }
```

### dm
Send a direct message to a specific user.
```json
{ "type": "dm", "to": "<user_uuid>", "content": "Hey!" }
```

### typing
Broadcast a typing indicator to all members of a room.
```json
{ "type": "typing", "room_id": "<uuid>", "typing": true }
```

---

## Server → Client Messages

### message
A new room message (including your own, echoed back via Kafka).
```json
{
  "type": "message",
  "payload": {
    "id": "<uuid>", "room_id": "<uuid>",
    "sender_id": "<uuid>", "content": "Hello everyone!",
    "created_at": "2026-03-04T13:00:00Z"
  }
}
```

### dm
A direct message sent to you (or by you, echoed back).
```json
{
  "type": "dm",
  "payload": {
    "id": "<uuid>", "sender_id": "<uuid>",
    "receiver_id": "<uuid>", "content": "Hey!",
    "created_at": "2026-03-04T13:00:00Z"
  }
}
```

### typing
A user started or stopped typing in a room.
```json
{
  "type": "typing",
  "payload": { "user_id": "<uuid>", "room_id": "<uuid>", "typing": true }
}
```

### presence
A user came online or went offline.
```json
{
  "type": "presence",
  "payload": { "user_id": "<uuid>", "status": "online" }
}
```

---

## Keep-Alive

The server sends a WebSocket `ping` frame every ~54 seconds. Clients must respond with a `pong`. Connections that do not pong within 60 seconds are closed.
