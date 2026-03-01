-- name: CreateUser :one
INSERT INTO users (username, email, password_hash, is_instance_admin, email_verified)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1 AND deleted_at IS NULL;

-- name: GetUserByUsername :one
SELECT * FROM users
WHERE username = $1 AND deleted_at IS NULL;

-- name: UpdateUser :one
UPDATE users
SET username = COALESCE(sqlc.narg(username), username),
    avatar_url = COALESCE(sqlc.narg(avatar_url), avatar_url),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $2,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: VerifyUserEmail :exec
UPDATE users
SET email_verified = TRUE,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteUser :exec
UPDATE users
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: CountUsers :one
SELECT COUNT(*) FROM users WHERE deleted_at IS NULL;

-- name: IsFirstUser :one
SELECT NOT EXISTS (SELECT 1 FROM users WHERE deleted_at IS NULL);

-- name: UpdateUserAvatar :exec
UPDATE users
SET avatar_url = $2,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: RemoveUserAvatar :exec
UPDATE users
SET avatar_url = NULL,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;
