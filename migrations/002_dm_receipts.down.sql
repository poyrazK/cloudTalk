DROP INDEX IF EXISTS idx_dm_receiver_read;
DROP INDEX IF EXISTS idx_dm_receiver_delivered;

ALTER TABLE direct_messages
  DROP COLUMN IF EXISTS read_at,
  DROP COLUMN IF EXISTS delivered_at;
