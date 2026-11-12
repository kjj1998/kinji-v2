-- bank_sms events come from the Shortcuts Message automation.
-- It captures in-app and online Apple Pay, which the Wallet trigger misses.
-- The source CHECK from 00001 has to include the new value.

-- +goose Up
ALTER TABLE capture_events
    DROP CONSTRAINT capture_events_source_check,
    ADD CONSTRAINT capture_events_source_check CHECK (source IN (
        'wallet_tap', 'wallet_notification', 
        'bank_notification', 'bank_sms'
    ));

-- +goose Down
-- Fails if any bank_sms rows exist: a rollback must not delete captured events
ALTER TABLE capture_events
DROP CONSTRAINT capture_events_source_check,
ADD CONSTRAINT capture_events_source_check CHECK (source IN ('wallet_tap', 'wallet_notification', 'bank_notification'));
