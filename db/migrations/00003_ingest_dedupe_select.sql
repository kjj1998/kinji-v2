-- ingest inserts with ON CONFLICT (dedupe_key) DO NOTHING. Postgres reads the existing
-- dedupe_key values to detect the conflict, which needs SELECT on that column on top of
-- INSERT. 00002 only granted SELECT (id), for RETURNING id, so every insert was denied.

-- +goose Up
GRANT SELECT (dedupe_key) ON capture_events TO ingest;

-- +goose Down
REVOKE SELECT (dedupe_key) ON capture_events FROM ingest;
