ALTER TABLE room_members
ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'member';

UPDATE room_members rm
SET role = 'owner'
FROM rooms r
WHERE r.id = rm.room_id
  AND r.created_by = rm.user_id;

WITH ownerless_rooms AS (
  SELECT room_id, user_id AS promoted_user_id
  FROM (
    SELECT rm.room_id,
           rm.user_id,
           ROW_NUMBER() OVER (PARTITION BY rm.room_id ORDER BY rm.joined_at ASC, rm.user_id ASC) AS row_num
    FROM room_members rm
    WHERE NOT EXISTS (
      SELECT 1
      FROM room_members existing_owner
      WHERE existing_owner.room_id = rm.room_id
        AND existing_owner.role = 'owner'
    )
  ) ranked_members
  WHERE row_num = 1
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
