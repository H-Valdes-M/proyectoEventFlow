CREATE TABLE event_processing (
    consumer_name TEXT NOT NULL CHECK (char_length(consumer_name) BETWEEN 1 AND 100 AND btrim(consumer_name) <> ''),
    event_id UUID NOT NULL,
    order_id UUID NOT NULL REFERENCES orders (id),
    status TEXT NOT NULL CHECK (status IN ('ready', 'retry_scheduled', 'completed', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0 AND attempts <= 4),
    next_attempt_at TIMESTAMPTZ,
    last_error_sanitized TEXT CHECK (char_length(last_error_sanitized) <= 500 AND last_error_sanitized = btrim(last_error_sanitized)),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (consumer_name, event_id)
);

CREATE INDEX event_processing_ready_idx ON event_processing (next_attempt_at) WHERE status = 'retry_scheduled';
CREATE INDEX event_processing_order_id_idx ON event_processing (order_id);

CREATE TABLE notifications (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES orders (id),
    notification_type TEXT NOT NULL CHECK (char_length(notification_type) BETWEEN 1 AND 100 AND btrim(notification_type) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (order_id, notification_type)
);
