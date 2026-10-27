-- +goose Up
-- Shaped as Better Auth's user model (plus its username plugin), mapped to snake_case in the
-- web app's auth config. Passwords and social logins live in Better Auth's account table, which
-- arrives with the web app, so adding providers later needs no change here.
CREATE TABLE users (
    id               uuid        PRIMARY KEY DEFAULT uuidv7(),
    name             text        NOT NULL,
    email            text        NOT NULL UNIQUE,
    email_verified   boolean     NOT NULL DEFAULT false,
    image            text,
    -- Better Auth stores the lowercased username here and the original casing in display_username.
    username         text        UNIQUE,
    display_username text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE api_tokens (
    id           uuid        PRIMARY KEY DEFAULT uuidv7(),
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- SHA-256 of the bearer token. The plaintext token is shown once and never stored.
    token_hash   bytea       NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    scope        text        NOT NULL CHECK (scope IN ('ingest')),
    name         text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at   timestamptz
);

CREATE INDEX api_tokens_user_id_idx ON api_tokens (user_id);

CREATE TABLE capture_events (
    -- UUID v7, so ids sort by arrival time.
    id           uuid        PRIMARY KEY DEFAULT uuidv7(),
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    source       text        NOT NULL
                             CHECK (source IN ('wallet_tap', 'wallet_notification', 'bank_notification')),
    -- Exactly as received. All parsing happens in enrich.
    payload      jsonb       NOT NULL,
    -- SHA-256 of user, source and the source's identifying fields (minute-truncated time).
    dedupe_key   bytea       NOT NULL UNIQUE CHECK (octet_length(dedupe_key) = 32),
    status       text        NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending', 'processing', 'done', 'failed', 'discarded')),
    attempts     integer     NOT NULL DEFAULT 0,
    last_error   text,
    received_at  timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz
);

CREATE INDEX capture_events_user_received_idx ON capture_events (user_id, received_at DESC);

-- Sweeper scans for events stuck in pending.
CREATE INDEX capture_events_pending_idx ON capture_events (received_at) WHERE status = 'pending';

-- +goose Down
DROP TABLE capture_events;
DROP TABLE api_tokens;
DROP TABLE users;
