-- name: CreateAPIToken :one
INSERT INTO api_tokens (user_id, token_hash, scope, name)
VALUES (
    sqlc.arg(user_id),
    sqlc.arg(token_hash),
    sqlc.arg(scope),
    sqlc.arg(name)
)
RETURNING id, created_at;

-- name: RevokeAPIToken :execrows
UPDATE api_tokens
SET
    revoked_at = now()
WHERE id = sqlc.arg(id)
    AND revoked_at is NULL;