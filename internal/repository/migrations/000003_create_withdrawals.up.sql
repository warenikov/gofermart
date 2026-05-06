CREATE TABLE IF NOT EXISTS withdrawals (
    id            BIGSERIAL    PRIMARY KEY,
    user_id       BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_number  TEXT         NOT NULL,
    sum           NUMERIC(15,2) NOT NULL CHECK (sum > 0),
    processed_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_withdrawals_user_id ON withdrawals(user_id);
