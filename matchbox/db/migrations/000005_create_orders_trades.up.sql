CREATE TABLE IF NOT EXISTS orders (
    id UUID PRIMARY KEY,
    user_id VARCHAR(50) NOT NULL,
    symbol VARCHAR(20) NOT NULL,
    side VARCHAR(10) NOT NULL,
    type VARCHAR(20) NOT NULL,
    price NUMERIC(24, 8) NOT NULL,
    quantity NUMERIC(24, 8) NOT NULL,
    filled_qty NUMERIC(24, 8) NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL, -- PENDING, PARTIALLY_FILLED, FILLED, CANCELED
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Index for cursor-based pagination (keyset pagination)
CREATE INDEX idx_orders_user_status_created ON orders (user_id, status, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS trades (
    id UUID PRIMARY KEY,
    maker_order_id UUID NOT NULL REFERENCES orders(id),
    taker_order_id UUID NOT NULL REFERENCES orders(id),
    symbol VARCHAR(20) NOT NULL,
    price NUMERIC(24, 8) NOT NULL,
    quantity NUMERIC(24, 8) NOT NULL,
    executed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_trades_symbol_executed ON trades (symbol, executed_at DESC, id DESC);
CREATE INDEX idx_trades_maker ON trades (maker_order_id);
CREATE INDEX idx_trades_taker ON trades (taker_order_id);
