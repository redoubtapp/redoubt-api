-- name: CreateMessageAttachment :one
INSERT INTO message_attachments (
    message_id,
    media_file_id,
    filename,
    display_order
) VALUES (
    $1, $2, $3, $4
) RETURNING *;

-- name: GetAttachmentByID :one
SELECT * FROM message_attachments
WHERE id = $1;

-- name: GetMessageAttachments :many
SELECT
    ma.id,
    ma.message_id,
    ma.media_file_id,
    ma.filename,
    ma.display_order,
    ma.created_at,
    mf.content_type,
    mf.size_bytes,
    mf.s3_key,
    mf.encryption_key,
    mf.encryption_iv
FROM message_attachments ma
JOIN media_files mf ON mf.id = ma.media_file_id
WHERE ma.message_id = $1
ORDER BY ma.display_order ASC;

-- name: GetMessageAttachmentsByMessages :many
SELECT
    ma.id,
    ma.message_id,
    ma.media_file_id,
    ma.filename,
    ma.display_order,
    ma.created_at,
    mf.content_type,
    mf.size_bytes
FROM message_attachments ma
JOIN media_files mf ON mf.id = ma.media_file_id
WHERE ma.message_id = ANY(@message_ids::uuid[])
ORDER BY ma.message_id, ma.display_order ASC;

-- name: DeleteMessageAttachment :exec
DELETE FROM message_attachments
WHERE id = $1;

-- name: DeleteMessageAttachments :exec
DELETE FROM message_attachments
WHERE message_id = $1;

-- name: CountMessageAttachments :one
SELECT COUNT(*)::int FROM message_attachments
WHERE message_id = $1;

-- name: CreateMediaFileWithMessage :one
INSERT INTO media_files (
    owner_id,
    message_id,
    s3_key,
    encryption_key,
    encryption_iv,
    content_type,
    size_bytes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetMediaFileForAttachment :one
SELECT mf.* FROM media_files mf
JOIN message_attachments ma ON ma.media_file_id = mf.id
WHERE ma.id = $1;
