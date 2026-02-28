-- name: SetTypingIndicator :exec
INSERT INTO typing_indicators (user_id, channel_id, started_at)
VALUES ($1, $2, NOW())
ON CONFLICT (user_id, channel_id)
DO UPDATE SET started_at = NOW();

-- name: ClearTypingIndicator :exec
DELETE FROM typing_indicators
WHERE user_id = $1 AND channel_id = $2;

-- name: ClearTypingIndicatorsByUserID :exec
DELETE FROM typing_indicators
WHERE user_id = $1;

-- name: GetTypingIndicatorsByChannelID :many
SELECT ti.user_id, ti.started_at, u.username
FROM typing_indicators ti
JOIN users u ON ti.user_id = u.id
WHERE ti.channel_id = $1 AND ti.started_at > $2
ORDER BY ti.started_at;

-- name: ClearStaleTypingIndicators :exec
DELETE FROM typing_indicators
WHERE started_at < $1;
