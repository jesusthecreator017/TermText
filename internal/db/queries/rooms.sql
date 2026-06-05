-- name: CreateRoom :one
INSERT INTO rooms (name, kind, created_by)
VALUES ($1, 'room', $2)
RETURNING *;

-- name: CreateDMRoom :one
INSERT INTO rooms (name, kind, dm_key, created_by)
VALUES ('', 'dm', $1, $2)
RETURNING *;

-- name: GetRoomByID :one
SELECT * FROM rooms
WHERE id = $1;

-- name: GetDMByKey :one
SELECT * FROM rooms
WHERE dm_key = $1;

-- name: AddRoomMember :exec
INSERT INTO room_members (room_id, user_id)
VALUES ($1, $2)
ON CONFLICT (room_id, user_id) DO NOTHING;

-- name: IsRoomMember :one
SELECT EXISTS (
    SELECT 1 FROM room_members
    WHERE room_id = $1 AND user_id = $2
) AS is_member;

-- name: ListRoomMemberIDs :many
SELECT user_id FROM room_members
WHERE room_id = $1;

-- name: ListRoomPeers :many
SELECT DISTINCT u.id, u.username
FROM room_members me
JOIN room_members other ON other.room_id = me.room_id AND other.user_id <> me.user_id
JOIN users u ON u.id = other.user_id
WHERE me.user_id = $1;

-- name: ListRoomsForUser :many
SELECT
    r.id,
    r.kind,
    (CASE
        WHEN r.kind = 'dm' THEN COALESCE((
            SELECT u.username
            FROM room_members rm2
            JOIN users u ON u.id = rm2.user_id
            WHERE rm2.room_id = r.id AND rm2.user_id <> $1
            LIMIT 1
        ), 'dm')
        ELSE r.name
    END)::text AS display_name,
    (
        SELECT rm3.user_id
        FROM room_members rm3
        WHERE rm3.room_id = r.id AND rm3.user_id <> $1
        LIMIT 1
    ) AS peer_id
FROM rooms r
JOIN room_members rm ON rm.room_id = r.id
WHERE rm.user_id = $1
ORDER BY r.created_at;
