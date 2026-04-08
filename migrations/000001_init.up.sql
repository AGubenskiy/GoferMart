CREATE TABLE IF NOT EXISTS users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    login VARCHAR(200) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS orders (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number VARCHAR(200) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(200) NOT NULL,
    accrual NUMERIC(20,2),
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT orders_status_check CHECK (status IN ('NEW', 'PROCESSING', 'INVALID', 'PROCESSED'))
);

CREATE INDEX IF NOT EXISTS orders_user_uploaded_at_idx
    ON orders (user_id, uploaded_at DESC);

CREATE INDEX IF NOT EXISTS orders_status_uploaded_at_idx
    ON orders (status, uploaded_at ASC);

CREATE TABLE IF NOT EXISTS withdrawals (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_number VARCHAR(200) NOT NULL UNIQUE,
    amount NUMERIC(20,2) NOT NULL CHECK (amount > 0),
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS withdrawals_user_processed_at_idx
    ON withdrawals (user_id, processed_at DESC);
