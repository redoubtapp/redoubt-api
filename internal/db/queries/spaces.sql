-- name: CreateSpace :one
INSERT INTO spaces (name, icon_url, owner_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetSpaceByID :one
SELECT * FROM spaces
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListUserSpaces :many
SELECT s.* FROM spaces s
INNER JOIN memberships m ON s.id = m.space_id
WHERE m.user_id = $1 AND s.deleted_at IS NULL
ORDER BY s.name;

-- name: UpdateSpace :one
UPDATE spaces
SET name = COALESCE(sqlc.narg(name), name),
    icon_url = COALESCE(sqlc.narg(icon_url), icon_url),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteSpace :exec
UPDATE spaces
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: CountSpaces :one
SELECT COUNT(*) FROM spaces WHERE deleted_at IS NULL;
