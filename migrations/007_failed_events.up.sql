CREATE TABLE failed_events (
    consumer_name TEXT NOT NULL CHECK (char_length(consumer_name) BETWEEN 1 AND 100 AND btrim(consumer_name) <> ''),
    event_id UUID NOT NULL,
    order_id UUID NOT NULL REFERENCES orders (id),
    reason_sanitized TEXT NOT NULL CHECK (char_length(reason_sanitized) BETWEEN 1 AND 500 AND reason_sanitized = btrim(reason_sanitized)),
    failed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (consumer_name, event_id)
);

CREATE INDEX failed_events_order_id_idx ON failed_events (order_id);

CREATE TABLE rejected_deliveries (
    consumer_name TEXT NOT NULL CHECK (char_length(consumer_name) BETWEEN 1 AND 100 AND btrim(consumer_name) <> ''),
    payload_digest CHAR(64) NOT NULL CHECK (payload_digest ~ '^[0-9a-f]{64}$'),
    reason_sanitized TEXT NOT NULL CHECK (char_length(reason_sanitized) BETWEEN 1 AND 500 AND reason_sanitized = btrim(reason_sanitized)),
    rejected_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (consumer_name, payload_digest)
);
