-- name: CreateMessage :one
INSERT INTO messages (room_id, user_id, body)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetMessageByID :one
SELECT * FROM messages
WHERE id = $1;

-- name: ListRecentMessages :many
SELECT m.id, m.room_id, m.user_id, m.body, m.created_at, u.username
FROM messages m
JOIN users u ON u.id = m.user_id
WHERE m.room_id = $1
ORDER BY m.created_at DESC, m.id DESC
LIMIT $2;

-- name: ListMessagesBefore :many
SELECT m.id, m.room_id, m.user_id, m.body, m.created_at, u.username
FROM messages m
JOIN users u ON u.id = m.user_id
WHERE m.room_id = $1
  AND (m.created_at < $2 OR (m.created_at = $2 AND m.id < $3))
ORDER BY m.created_at DESC, m.id DESC
LIMIT $4;

-- name: UpdateLastRead :exec
UPDATE room_members
SET last_read_message_id = $3
WHERE room_id = $1 AND user_id = $2;

-- name: SearchMessages :many
SELECT m.id, m.room_id, m.user_id, m.body, m.created_at, u.username
FROM messages m
JOIN users u ON u.id = m.user_id
JOIN room_members rm ON rm.room_id = m.room_id AND rm.user_id = $1
WHERE m.search_tsv @@ plainto_tsquery('english', $2)
ORDER BY ts_rank(m.search_tsv, plainto_tsquery('english', $2)) DESC, m.created_at DESC
LIMIT 50;
