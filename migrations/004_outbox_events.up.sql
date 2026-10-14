CREATE TABLE outbox_events (
    message_id UUID PRIMARY KEY,
    event_id UUID NOT NULL,
    event_type TEXT NOT NULL CHECK (char_length(event_type) BETWEEN 1 AND 100),
    order_id UUID NOT NULL REFERENCES orders (id),
    payload JSONB NOT NULL,
    available_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ,
    publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
    last_publish_error TEXT,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX outbox_events_pending_idx ON outbox_events (available_at) WHERE published_at IS NULL;
CREATE INDEX outbox_events_order_id_idx ON outbox_events (order_id);
