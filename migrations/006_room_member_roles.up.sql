ALTER TABLE room_members
ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'member';

UPDATE room_members rm
SET role = 'owner'
FROM rooms r
WHERE r.id = rm.room_id
  AND r.created_by = rm.user_id;

ALTER TABLE room_members
DROP CONSTRAINT IF EXISTS room_members_role_check;

ALTER TABLE room_members
ADD CONSTRAINT room_members_role_check CHECK (role IN ('owner', 'member'));
