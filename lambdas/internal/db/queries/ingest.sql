-- name: GetActiveTokenByHash :one
SELECT user_id, id
FROM api_tokens
WHERE token_hash = sqlc.arg(hash) AND scope = 'ingest' AND revoked_at IS NULL;

-- name: InsertCaptureEvent :one
INSERT INTO capture_events (user_id, source, payload, dedupe_key)
VALUES (
    sqlc.arg(user_id),
    sqlc.arg(source),
    sqlc.arg(payload),
    sqlc.arg(dedupe_key)
)
ON CONFLICT (dedupe_key) DO NOTHING
RETURNING id;

-- name: TouchAPIToken :exec
UPDATE api_tokens
SET last_used_at = now()
WHERE id = sqlc.arg(id);