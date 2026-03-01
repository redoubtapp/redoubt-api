-- name: CreateWSConnection :one
INSERT INTO ws_connections (user_id, connection_id, server_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetWSConnectionByID :one
SELECT * FROM ws_connections
WHERE connection_id = $1;

-- name: GetWSConnectionsByUserID :many
SELECT * FROM ws_connections
WHERE user_id = $1
ORDER BY connected_at;

-- name: GetWSConnectionsByServerID :many
SELECT * FROM ws_connections
WHERE server_id = $1
ORDER BY connected_at;

-- name: CountWSConnectionsByUserID :one
SELECT COUNT(*) FROM ws_connections
WHERE user_id = $1;

-- name: UpdateWSConnectionPing :exec
UPDATE ws_connections
SET last_ping_at = NOW()
WHERE connection_id = $1;

-- name: DeleteWSConnection :exec
DELETE FROM ws_connections
WHERE connection_id = $1;

-- name: DeleteWSConnectionsByUserID :exec
DELETE FROM ws_connections
WHERE user_id = $1;

-- name: DeleteWSConnectionsByServerID :exec
DELETE FROM ws_connections
WHERE server_id = $1;

-- name: DeleteStaleWSConnections :exec
DELETE FROM ws_connections
WHERE last_ping_at < $1;
