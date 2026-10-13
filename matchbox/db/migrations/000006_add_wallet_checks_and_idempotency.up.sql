ALTER TABLE wallets
ADD CONSTRAINT chk_locked_balance_non_negative CHECK (locked_balance >= 0),
ADD CONSTRAINT chk_available_balance_non_negative CHECK (available_balance >= 0);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    key VARCHAR(255) PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
