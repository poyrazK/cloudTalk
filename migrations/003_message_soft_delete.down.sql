DROP INDEX IF EXISTS idx_dm_receiver_deleted;
DROP INDEX IF EXISTS idx_messages_room_deleted;

ALTER TABLE direct_messages
  DROP COLUMN IF EXISTS deleted_at,
  DROP COLUMN IF EXISTS edited_at;

ALTER TABLE messages
  DROP COLUMN IF EXISTS deleted_at,
  DROP COLUMN IF EXISTS edited_at;
