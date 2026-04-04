ALTER TABLE room_members
ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'member';

UPDATE room_members rm
SET role = 'owner'
FROM rooms r
WHERE r.id = rm.room_id
  AND r.created_by = rm.user_id;

WITH ownerless_rooms AS (
  SELECT rm.room_id, MIN(rm.user_id) AS promoted_user_id
  FROM room_members rm
  WHERE NOT EXISTS (
    SELECT 1
    FROM room_members existing_owner
    WHERE existing_owner.room_id = rm.room_id
      AND existing_owner.role = 'owner'
  )
  GROUP BY rm.room_id
)
UPDATE room_members rm
SET role = 'owner'
FROM ownerless_rooms oroom
WHERE rm.room_id = oroom.room_id
  AND rm.user_id = oroom.promoted_user_id;

ALTER TABLE room_members
DROP CONSTRAINT IF EXISTS room_members_role_check;

ALTER TABLE room_members
ADD CONSTRAINT room_members_role_check CHECK (role IN ('owner', 'member'));
