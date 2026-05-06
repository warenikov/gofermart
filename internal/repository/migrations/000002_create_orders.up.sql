CREATE TABLE IF NOT EXISTS orders (
    number      TEXT          PRIMARY KEY,
    user_id     BIGINT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status      VARCHAR(16)   NOT NULL DEFAULT 'NEW'
                              CHECK (status IN ('NEW', 'PROCESSING', 'INVALID', 'PROCESSED')),
    accrual     NUMERIC(15,2),
    uploaded_at TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id);
CREATE INDEX IF NOT EXISTS idx_orders_status  ON orders(status) WHERE status IN ('NEW', 'PROCESSING');
