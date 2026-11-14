-- +goose Up
-- A category's type decides how its transactions count in totals: expense -> total spent,
-- income -> total income and net savings, credit (cashback, interest) -> net savings only,
-- transfer -> excluded everywhere. Transactions get their type from their category, never
-- store their own, so the two cannot disagree.
CREATE TABLE categories (
    name            text         PRIMARY KEY,
    description     text         NOT NULL,
    type            text         NOT NULL DEFAULT 'expense' CHECK (type IN ('expense', 'income', 'credit', 'transfer')),
    created_at      timestamptz  NOT NULL DEFAULT now()
);

-- Kinji's 14 categories, plus Travel. Descriptions are read by the enrich LLM prompt; edit them (as the owner
-- role) to tune categorization, no migration needed. Cues taken from Kinji's parser prompt.
-- Categorize by what the money was for, not how it moved: PayNow and refunds take the category of
-- what they paid for (a refund or a friend paying back their share is an inflow in that category).
INSERT INTO categories (name, type, description) VALUES
    ('Entertainment', 'expense',  'Cinemas, games, concerts, events and other leisure activities.'),
    ('Food',          'expense',  'Restaurants, cafes, hawker centres, food courts and food delivery. Not supermarkets (Groceries).'),
    ('Groceries',     'expense',  'Supermarkets, grocery stores and wet markets.'),
    ('Health',        'expense',  'Clinics, doctors, dentists, pharmacies, fitness and gyms.'),
    ('Income',        'income',   'Incoming salary and other earned income.'),
    ('Shopping',      'expense',  'General retail and online shopping (e.g. Shopee, Lazada) not covered by another category.'),
    ('Subscriptions', 'expense',  'Recurring streaming services, SaaS and app subscriptions.'),
    ('Transport',     'expense',  'Ride-hailing, public transit, EZ-Link and SimplyGo top-ups, fuel and parking.'),
    ('Utilities',     'expense',  'Electricity, water, gas, telco and internet bills.'),
    ('Credit',        'credit',   'Money back rather than earned: bank interest, rebates, cashback and card credits. Not refunds (those go to the original category).'),
    ('Transfer',      'transfer', 'Money moved between own accounts, loans to or from other people, and credit card bill payments. Never counted as spending.'),
    ('Taxes',         'expense',  'Any kind of tax payment, e.g. IRAS income or property tax.'),
    ('Accommodation', 'expense',  'Hotels, Airbnb, rentals and other lodging bookings. Flights go to Travel.'),
    ('Travel',        'expense',  'Airfare (e.g. SIA, Scoot, Jetstar), flights booked via Trip.com or Expedia, tour packages and travel agencies. Not lodging (Accommodation). Spending during a trip goes to its own category, e.g. a meal abroad is Food.'),
    ('Insurance',     'expense',  'Insurance premiums: life, health, car, home and travel cover.');

-- One row per real purchase, refund or transfer. Several capture events can feed one
-- transaction (see transaction_sources).
CREATE TABLE transactions (
    id               uuid        PRIMARY KEY DEFAULT uuidv7(),
    user_id          uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Best estimate of purchase time (e.g. SMS date plus received_at), stored in UTC.
    occurred_at      timestamptz NOT NULL,
    -- As the source wrote it; NULL when the Wallet trigger sends a blank merchant (needs_review).
    merchant_raw     text,
    -- Cleaned-up name, from merchant_aliases or the LLM.
    merchant         text,
    -- Always positive; direction gives the sign. 0 is allowed (Wallet sometimes sends 0.0; needs_review).
    amount_cents     bigint      NOT NULL CHECK (amount_cents >= 0),
    currency         text        NOT NULL DEFAULT 'SGD' CHECK (currency ~ '^[A-Z]{3}$'),
    direction        text        NOT NULL CHECK (direction IN ('inflow', 'outflow')),
    -- NULL until categorized. Type comes from the category (JOIN categories).
    category         text        REFERENCES categories (name) ON UPDATE CASCADE,
    -- Card's last four digits, used for cross-source matching.
    card             text,
    status           text        NOT NULL DEFAULT 'provisional' CHECK (status IN ('provisional', 'confirmed', 'flagged', 'needs_review')),
    -- LLM confidence; NULL when no LLM call was made (merchant found in merchant_aliases).
    confidence       real        CHECK (confidence BETWEEN 0 AND 1),
    notes            text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

-- Dashboard listing, and cross-source matching (one user's transactions in a time window).
CREATE INDEX transactions_user_occurred_idx ON transactions (user_id, occurred_at DESC);

-- Links capture events to transactions, many to one: one purchase can arrive as a Wallet tap,
-- a notification and a bank SMS. capture_event_id is the primary key, so an event links at most
-- once; enrich inserts with ON CONFLICT (capture_event_id) DO NOTHING, which makes SQS
-- redelivery harmless.
CREATE TABLE transaction_sources (
    capture_event_id    uuid            PRIMARY KEY REFERENCES capture_events (id) ON DELETE CASCADE,
    transaction_id      uuid            NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
    created_at          timestamptz     NOT NULL DEFAULT now()
);

-- The events behind a transaction (the primary key only covers lookups by event).
CREATE INDEX transaction_sources_transaction_idx ON transaction_sources (transaction_id);

-- Remembers how a raw merchant name maps to a merchant and category, so enrich only calls the
-- LLM for merchants it has not seen. Enrich upserts on (user_id, raw_name); a 'user' row (a
-- correction from the review inbox) must never be overwritten by an 'llm' guess.
CREATE TABLE merchant_aliases (
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Normalized in Go before every lookup and insert (uppercase, spaces trimmed and collapsed),
    -- so "Shopee SG MP " and "SHOPEE SG MP" share one alias.
    raw_name        text        NOT NULL,
    merchant        text        NOT NULL,
    category        text        NOT NULL REFERENCES categories (name) ON UPDATE CASCADE,
    origin          text        NOT NULL CHECK (origin IN ('user','llm')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, raw_name)
);

-- +goose Down
DROP TABLE transaction_sources;
DROP TABLE merchant_aliases;
DROP TABLE transactions;
DROP TABLE categories;
