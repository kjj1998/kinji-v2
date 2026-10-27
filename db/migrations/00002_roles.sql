-- Roles are created NOLOGIN so no password lands in git. Enable each one by hand:
--   ALTER ROLE <role> WITH LOGIN PASSWORD '<generated>';
-- Created with SQL rather than the Neon console, since console roles join neon_superuser.
-- Migrations run as the owner role for now; a separate migrator role can come later.

-- +goose Up
CREATE ROLE ingest NOLOGIN;
GRANT USAGE ON SCHEMA public TO ingest;
-- Look up a token by hash and record when it was last used.
GRANT SELECT ON api_tokens TO ingest;
GRANT UPDATE (last_used_at) ON api_tokens TO ingest;
-- Insert raw events (ON CONFLICT DO NOTHING); SELECT (id) is for INSERT ... RETURNING id.
GRANT INSERT ON capture_events TO ingest;
GRANT SELECT (id) ON capture_events TO ingest;

-- Bootstrap CLI that creates a user and issues ingest tokens until the web app exists.
CREATE ROLE mint_token NOLOGIN;
GRANT USAGE ON SCHEMA public TO mint_token;
GRANT SELECT, INSERT ON users TO mint_token;
GRANT SELECT, INSERT ON api_tokens TO mint_token;
GRANT UPDATE (revoked_at) ON api_tokens TO mint_token;

-- +goose Down
REVOKE ALL ON api_tokens, users FROM mint_token;
REVOKE ALL ON SCHEMA public FROM mint_token;
DROP ROLE mint_token;

REVOKE ALL ON capture_events, api_tokens FROM ingest;
REVOKE ALL ON SCHEMA public FROM ingest;
DROP ROLE ingest;
