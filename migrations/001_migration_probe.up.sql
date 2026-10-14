CREATE TABLE eventflow_migration_probe (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
