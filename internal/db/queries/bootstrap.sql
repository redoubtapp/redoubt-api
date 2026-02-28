-- name: GetBootstrapState :one
SELECT * FROM bootstrap_state WHERE id = 1;

-- name: SetBootstrapInitialized :exec
UPDATE bootstrap_state
SET initialized = TRUE,
    invite_code = $1,
    initialized_at = NOW()
WHERE id = 1;

-- name: IsBootstrapInitialized :one
SELECT initialized FROM bootstrap_state WHERE id = 1;
