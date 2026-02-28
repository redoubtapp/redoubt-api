-- name: CreateInvite :one
INSERT INTO invites (code, space_id, created_by, max_uses, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetInviteByCode :one
SELECT * FROM invites
WHERE code = $1
  AND revoked_at IS NULL
  AND (max_uses IS NULL OR uses < max_uses)
  AND (expires_at IS NULL OR expires_at > NOW());

-- name: GetInviteByID :one
SELECT * FROM invites
WHERE id = $1;

-- name: ListSpaceInvites :many
SELECT i.*, u.username as created_by_username
FROM invites i
INNER JOIN users u ON i.created_by = u.id
WHERE i.space_id = $1 AND i.revoked_at IS NULL
ORDER BY i.created_at DESC;

-- name: IncrementInviteUses :exec
UPDATE invites
SET uses = uses + 1
WHERE id = $1;

-- name: RevokeInvite :exec
UPDATE invites
SET revoked_at = NOW()
WHERE id = $1 AND revoked_at IS NULL;

-- name: GetInviteWithSpaceInfo :one
SELECT i.*, s.name as space_name, s.icon_url as space_icon_url
FROM invites i
INNER JOIN spaces s ON i.space_id = s.id
WHERE i.code = $1
  AND i.revoked_at IS NULL
  AND s.deleted_at IS NULL
  AND (i.max_uses IS NULL OR i.uses < i.max_uses)
  AND (i.expires_at IS NULL OR i.expires_at > NOW());
