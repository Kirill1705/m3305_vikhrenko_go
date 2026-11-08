-- +goose Up
CREATE TABLE IF NOT EXISTS trip_idempotency_keys (
    key          UUID PRIMARY KEY,
    request_hash BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    response     JSONB CHECK (response IS NULL OR jsonb_typeof(response) = 'object'),
    expires_at   TIMESTAMPTZ NOT NULL
);


-- +goose Down
DROP TABLE IF NOT EXISTS trip_idempotency_keys;
