CREATE TABLE orders (
    id UUID PRIMARY KEY,
    customer_id TEXT NOT NULL CHECK (char_length(customer_id) BETWEEN 1 AND 100 AND btrim(customer_id) <> ''),
    total_minor BIGINT NOT NULL CHECK (total_minor > 0),
    status TEXT NOT NULL CHECK (status IN ('pending', 'processed', 'failed')),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE order_items (
    order_id UUID NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    product_id TEXT NOT NULL CHECK (char_length(product_id) BETWEEN 1 AND 100 AND btrim(product_id) <> ''),
    quantity BIGINT NOT NULL CHECK (quantity BETWEEN 1 AND 1000000),
    unit_price_minor BIGINT NOT NULL CHECK (unit_price_minor > 0),
    PRIMARY KEY (order_id, product_id)
);
