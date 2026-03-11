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

If the sender is not a room member, the server rejects the message and does not broadcast it.

### dm
Send a direct message to a specific user.
```json
{ "type": "dm", "to": "<user_uuid>", "content": "Hey!" }
```

### read_dm
Mark a DM as read. Only the receiver of the DM can do this.
```json
{ "type": "read_dm", "dm_id": "<dm_uuid>" }
```

### read_room
Mark a room as read for the current user.
```json
{ "type": "read_room", "room_id": "<room_uuid>" }
```

### edit_message
Edit a room message. Only the sender can edit.
```json
{ "type": "edit_message", "message_id": "<uuid>", "content": "Updated text" }
```

### delete_message
Soft delete a room message. Only the sender can delete.
```json
{ "type": "delete_message", "message_id": "<uuid>" }
```

### edit_dm
Edit a direct message. Only the sender can edit.
```json
{ "type": "edit_dm", "dm_id": "<uuid>", "content": "Updated text" }
```

### delete_dm
Soft delete a direct message. Only the sender can delete.
```json
{ "type": "delete_dm", "dm_id": "<uuid>" }
```

Self DMs are rejected.

### typing
Broadcast a typing indicator to all members of a room.
```json
{ "type": "typing", "room_id": "<uuid>", "typing": true }
```

Behavior:

- sender must be a room member
- delivered to other subscribed room members
- not echoed back to sender

### typing_dm
Send a DM typing indicator to a specific user.
```json
{ "type": "typing_dm", "to": "<user_uuid>", "typing": true }
```

Behavior:

- delivered only to the DM recipient
- not echoed back to sender
- self-DM typing is rejected

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

### dm_receipt
Delivery/read receipt for a direct message.
```json
{
  "type": "dm_receipt",
  "payload": {
    "dm_id": "<uuid>",
    "sender_id": "<uuid>",
    "receiver_id": "<uuid>",
    "delivered_at": "2026-03-04T13:00:05Z",
    "read_at": "2026-03-04T13:01:10Z"
  }
}
```

`delivered_at` is set when the recipient WebSocket connection receives the DM event.
`read_at` is set when the recipient sends `read_dm`.

### message_updated / message_deleted
Room message update and soft-delete events.

### dm_updated / dm_deleted
Direct message update and soft-delete events.

### typing
A user started or stopped typing in a room.
```json
{
  "type": "typing",
  "payload": { "user_id": "<uuid>", "room_id": "<uuid>", "typing": true }
}
```

### typing_dm
A user started or stopped typing in a DM thread.
```json
{
  "type": "typing_dm",
  "payload": { "user_id": "<uuid>", "to_user_id": "<uuid>", "typing": true }
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
