# Messaging Semantics

This document defines how core chat message behavior works in cloudTalk.

## Scope

- Room messages
- Direct messages (DMs)
- Delivery/read receipts for DMs
- Unread counts for DMs
- Unread counts for rooms
- Message edit and soft delete

## Message lifecycle

### Sent

A message is considered sent when it is persisted in PostgreSQL.

### Delivered (DM only)

`delivered_at` is set when the recipient WebSocket connection successfully receives the DM event.

Notes:

- Delivery is connection-based, not "viewed by user".
- If recipient is offline, delivery is not marked yet.

### Read (DM only)

`read_at` is set when the recipient sends `read_dm` over WebSocket.

## Unread counts (DM)

Unread DM counts are grouped by sender user ID.

Count rule:

- `receiver_id = current_user`
- `read_at IS NULL`

API:

- `GET /api/v1/dms/unread-counts`

## Unread counts (room)

Unread room counts are grouped by room ID.

Count rules:

- user must be a room member
- message sender is not the current user
- message `created_at` is greater than read boundary

Read boundary rule:

- use `room_read_state.last_read_at` if present
- otherwise use membership `joined_at`

This ensures room backlog from before a join does not count unread.

APIs:

- `GET /api/v1/rooms/unread-counts`
- WebSocket action `read_room` to advance the boundary

## Conversation list (DM)

DM conversations are exposed as one row per partner user, ordered by latest message timestamp.

Each conversation item contains:

- `user_id`
- `username`
- `unread_count`
- `last_message`

API:

- `GET /api/v1/dms/conversations`

Query params:

- `limit` (1-100, default 50)

Conversation semantics:

- `last_message` is the latest DM between current user and partner.
- `unread_count` counts inbound DMs where `read_at IS NULL`.
- Soft-deleted messages can still appear as `last_message` with `deleted_at` set.

## Edit and soft delete

Both room messages and DMs support edit and soft delete.

### Ownership rules

- Only the original sender can edit.
- Only the original sender can soft delete.

### Soft delete behavior

- Deletion sets `deleted_at`.
- Rows remain in history (no hard delete).
- Deleted messages cannot be edited.

### Edit behavior

- Editing updates content and sets `edited_at`.
- Message ID remains unchanged.

## WebSocket actions

Client actions:

- `message`
- `dm`
- `read_dm`
- `read_room`
- `edit_message`
- `delete_message`
- `edit_dm`
- `delete_dm`

Server events:

- `message`
- `dm`
- `dm_receipt`
- `message_updated`
- `message_deleted`
- `dm_updated`
- `dm_deleted`

## Error semantics (high level)

Service layer returns typed domain errors for business cases such as:

- forbidden edit/delete
- message already deleted
- message not found

Handlers map these to protocol-appropriate responses/logging.

## Data fields

Room messages:

- `edited_at`
- `deleted_at`

Direct messages:

- `delivered_at`
- `read_at`
- `edited_at`
- `deleted_at`

## Future extensions

- Edit history/audit trail
- Multi-device per-user delivery semantics
- Hard-delete and retention policies
