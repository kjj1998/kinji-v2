-- enrich reads raw capture events and writes transactions, their source links and merchant aliases.
-- Created NOLOGIN so no password lands in git. Enable it by hand when step 15 needs it:
--   ALTER ROLE enrich WITH LOGIN PASSWORD '<generated>';
-- No DELETE anywhere: an event that is not a purchase is marked 'discarded', never removed.
-- No access to users or api_tokens.

-- +goose Up
CREATE ROLE enrich NOLOGIN;
GRANT USAGE ON SCHEMA public TO enrich;
-- Load the raw payload, then record progress. Column-level UPDATE keeps payload and dedupe_key
-- unchangeable (raw first); only ingest inserts events.
GRANT SELECT ON capture_events TO enrich;
GRANT UPDATE (status, attempts, last_error, processed_at) ON capture_events TO enrich;
-- Read the category list and descriptions for the LLM prompt; never edit them.
GRANT SELECT ON categories TO enrich;
-- Cross-source matching reads transactions; a later event can update an existing one.
GRANT SELECT, INSERT, UPDATE ON transactions TO enrich;
-- Links are never edited. SELECT also covers ON CONFLICT (capture_event_id).
GRANT SELECT, INSERT ON transaction_sources TO enrich;
-- Upsert on (user_id, raw_name): ON CONFLICT ... DO UPDATE needs SELECT, INSERT and UPDATE.
GRANT SELECT, INSERT, UPDATE ON merchant_aliases TO enrich;

-- +goose Down
-- Table-level REVOKE also removes the column-level UPDATE on capture_events.
REVOKE ALL ON capture_events, categories, transactions, transaction_sources, merchant_aliases FROM enrich;
REVOKE ALL ON SCHEMA public FROM enrich;
DROP ROLE enrich;
