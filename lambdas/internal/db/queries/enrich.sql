-- name: GetCaptureEvent :one
SELECT id, user_id, source, payload, status, attempts, received_at
FROM capture_events
WHERE id = sqlc.arg(id);

-- name: MarkEventProcessing :one
UPDATE capture_events
SET status = 'processing', attempts = attempts + 1
WHERE id = sqlc.arg(id) AND status IN ('pending', 'processing', 'failed')
RETURNING attempts;

-- name: ListCategories :many
SELECT name, description, type
FROM categories
ORDER BY name;

-- name: GetMerchantAlias :one
SELECT merchant, category, origin
FROM merchant_aliases
WHERE user_id = sqlc.arg(user_id) AND raw_name = sqlc.arg(raw_name);

-- name: LockCaptureEvent :one
SELECT status
FROM capture_events
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: InsertTransaction :one
INSERT INTO transactions (user_id, occurred_at, merchant_raw, merchant, amount_cents, currency, direction,
    category, card, status, confidence, notes)
VALUES (
    sqlc.arg(user_id),
    sqlc.arg(occurred_at),
    sqlc.narg(merchant_raw),
    sqlc.narg(merchant),
    sqlc.arg(amount_cents),
    sqlc.arg(currency),
    sqlc.arg(direction),
    sqlc.narg(category),
    sqlc.narg(card),
    sqlc.arg(status),
    sqlc.narg(confidence),
    sqlc.narg(notes)
)
RETURNING id;

-- name: InsertTransactionSource :execrows
INSERT INTO transaction_sources (capture_event_id, transaction_id)
VALUES (
    sqlc.arg(capture_event_id),
    sqlc.arg(transaction_id)
)
ON CONFLICT (capture_event_id) DO NOTHING;

-- name: UpsertMerchantAlias :exec
-- Remembers the LLM's first guess for a raw merchant name. An existing row, 'user' or 'llm',
-- is never overwritten: user corrections always win, and a merchant keeps one category.
-- raw_name must already be normalized in Go (same function as GetMerchantAlias).
INSERT INTO merchant_aliases (user_id, raw_name, merchant, category, origin)
VALUES (
    sqlc.arg(user_id),
    sqlc.arg(raw_name),
    sqlc.arg(merchant),
    sqlc.arg(category),
    'llm'
)
ON CONFLICT (user_id, raw_name) DO NOTHING;

-- name: MarkEventDone :exec
UPDATE capture_events
SET status = 'done', processed_at = now(), last_error = NULL
WHERE id = sqlc.arg(id);

-- name: MarkEventDiscarded :exec
UPDATE capture_events
SET status = 'discarded', processed_at = now(), last_error = sqlc.arg(error)
WHERE id = sqlc.arg(id);

-- name: MarkEventFailed :exec
UPDATE capture_events
SET status = 'failed', last_error = sqlc.arg(error)
WHERE id = sqlc.arg(id) AND status NOT IN ('done', 'discarded');