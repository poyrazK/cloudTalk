ALTER TABLE room_members
DROP CONSTRAINT IF EXISTS room_members_role_check;

ALTER TABLE room_members
DROP COLUMN IF EXISTS role;
