-- name: CreateChannel :one
INSERT INTO channels (space_id, name, type, position, max_participants)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetChannelByID :one
SELECT * FROM channels
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListSpaceChannels :many
SELECT * FROM channels
WHERE space_id = $1 AND deleted_at IS NULL
ORDER BY position, created_at;

-- name: UpdateChannel :one
UPDATE channels
SET name = COALESCE(sqlc.narg(name), name),
    type = COALESCE(sqlc.narg(type), type),
    position = COALESCE(sqlc.narg(position), position),
    max_participants = COALESCE(sqlc.narg(max_participants), max_participants),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: UpdateChannelPosition :exec
UPDATE channels
SET position = $2,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteChannel :exec
UPDATE channels
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetMaxChannelPosition :one
SELECT COALESCE(MAX(position), -1) FROM channels
WHERE space_id = $1 AND deleted_at IS NULL;

-- name: GetSpaceIDByChannelID :one
SELECT space_id FROM channels
WHERE id = $1 AND deleted_at IS NULL;
