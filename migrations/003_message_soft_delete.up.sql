ALTER TABLE messages
  ADD COLUMN IF NOT EXISTS edited_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE direct_messages
  ADD COLUMN IF NOT EXISTS edited_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_messages_room_deleted
  ON messages (room_id, deleted_at, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_dm_receiver_deleted
  ON direct_messages (receiver_id, deleted_at, created_at DESC);
