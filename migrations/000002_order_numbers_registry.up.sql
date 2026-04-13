-- noinspection SqlNoDataSourceInspection
CREATE TABLE IF NOT EXISTS order_numbers (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number VARCHAR(200) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind VARCHAR(200) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT order_numbers_kind_check CHECK (kind IN ('UPLOAD', 'WITHDRAWAL'))
);

CREATE INDEX IF NOT EXISTS order_numbers_user_created_at_idx
    ON order_numbers (user_id, created_at DESC);

INSERT INTO order_numbers (number, user_id, kind, created_at)
SELECT number, user_id, 'UPLOAD', uploaded_at
FROM orders
ON CONFLICT (number) DO NOTHING;

INSERT INTO order_numbers (number, user_id, kind, created_at)
SELECT order_number, user_id, 'WITHDRAWAL', processed_at
FROM withdrawals
ON CONFLICT (number) DO NOTHING;
