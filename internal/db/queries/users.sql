-- name: CreateUser :one
INSERT INTO users (username, password_hash)
VALUES ($1, $2)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1;

-- name: GetUserByUsername :one
SELECT * FROM users
WHERE lower(username) = lower($1);

-- name: SetUserPublicKey :exec
UPDATE users SET public_key = $2, updated_at = now()
WHERE id = $1;
