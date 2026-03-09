ALTER TABLE direct_messages
  ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS read_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_dm_receiver_delivered
  ON direct_messages (receiver_id, delivered_at);

CREATE INDEX IF NOT EXISTS idx_dm_receiver_read
  ON direct_messages (receiver_id, read_at);
