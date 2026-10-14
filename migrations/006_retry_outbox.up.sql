ALTER TABLE outbox_events
    ADD COLUMN consumer_name TEXT,
    ADD COLUMN attempt INTEGER NOT NULL DEFAULT 1 CHECK (attempt >= 1);

CREATE UNIQUE INDEX outbox_retry_attempt_uniq
    ON outbox_events (consumer_name, event_id, attempt)
    WHERE consumer_name IS NOT NULL;
