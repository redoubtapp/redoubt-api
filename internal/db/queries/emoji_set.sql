-- name: GetAllEmoji :many
SELECT * FROM emoji_set
ORDER BY category, sort_order;

-- name: GetEmojiByCategory :many
SELECT * FROM emoji_set
WHERE category = $1
ORDER BY sort_order;

-- name: IsValidEmoji :one
SELECT EXISTS (
    SELECT 1 FROM emoji_set WHERE emoji = $1
);

-- name: SearchEmoji :many
SELECT * FROM emoji_set
WHERE name ILIKE '%' || $1 || '%'
ORDER BY sort_order
LIMIT 20;

-- name: GetEmojiCategories :many
SELECT DISTINCT category FROM emoji_set
ORDER BY MIN(sort_order);
