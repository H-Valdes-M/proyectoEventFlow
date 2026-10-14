CREATE TABLE idempotency_keys (
    idempotency_key TEXT PRIMARY KEY CHECK (char_length(idempotency_key) BETWEEN 1 AND 128 AND idempotency_key !~ '[[:space:]]'),
    fingerprint CHAR(64) NOT NULL,
    order_id UUID NOT NULL REFERENCES orders (id) DEFERRABLE INITIALLY DEFERRED,
    response_status SMALLINT NOT NULL CHECK (response_status = 201),
    response_body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
