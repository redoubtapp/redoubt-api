-- name: CreateMediaFile :one
INSERT INTO media_files (
    owner_id,
    s3_key,
    encryption_key,
    encryption_iv,
    content_type,
    size_bytes
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetMediaFileByID :one
SELECT * FROM media_files
WHERE id = $1;

-- name: GetMediaFileByS3Key :one
SELECT * FROM media_files
WHERE s3_key = $1;

-- name: GetMediaFilesByOwner :many
SELECT * FROM media_files
WHERE owner_id = $1
ORDER BY created_at DESC;

-- name: DeleteMediaFile :exec
DELETE FROM media_files
WHERE id = $1;

-- name: DeleteMediaFileByS3Key :exec
DELETE FROM media_files
WHERE s3_key = $1;
