-- name: CreateVoiceConnection :one
INSERT INTO voice_connections (user_id, channel_id, space_id, livekit_room)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetVoiceConnectionByUserID :one
SELECT * FROM voice_connections
WHERE user_id = $1;

-- name: GetVoiceConnectionsByChannelID :many
SELECT vc.*, u.username, u.avatar_url
FROM voice_connections vc
JOIN users u ON vc.user_id = u.id
WHERE vc.channel_id = $1
ORDER BY vc.connected_at;

-- name: GetVoiceConnectionsBySpaceID :many
SELECT vc.*, u.username, u.avatar_url
FROM voice_connections vc
JOIN users u ON vc.user_id = u.id
WHERE vc.space_id = $1
ORDER BY vc.channel_id, vc.connected_at;

-- name: GetVoiceConnectionsByRoom :many
SELECT vc.*, u.username, u.avatar_url
FROM voice_connections vc
JOIN users u ON vc.user_id = u.id
WHERE vc.livekit_room = $1
ORDER BY vc.connected_at;

-- name: CountVoiceConnectionsByChannelID :one
SELECT COUNT(*) FROM voice_connections
WHERE channel_id = $1;

-- name: UpdateVoiceConnectionMuteState :exec
UPDATE voice_connections
SET self_muted = COALESCE(sqlc.narg(self_muted), self_muted),
    self_deafened = COALESCE(sqlc.narg(self_deafened), self_deafened),
    server_muted = COALESCE(sqlc.narg(server_muted), server_muted)
WHERE user_id = $1;

-- name: DeleteVoiceConnection :exec
DELETE FROM voice_connections
WHERE user_id = $1;

-- name: DeleteVoiceConnectionsByChannelID :exec
DELETE FROM voice_connections
WHERE channel_id = $1;

-- name: DeleteVoiceConnectionsByRoom :exec
DELETE FROM voice_connections
WHERE livekit_room = $1;
