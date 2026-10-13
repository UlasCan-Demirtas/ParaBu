CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE IF NOT EXISTS candlesticks (
    ts TIMESTAMPTZ NOT NULL,
    symbol VARCHAR(20) NOT NULL,
    interval VARCHAR(5) NOT NULL,
    open NUMERIC(20, 8) NOT NULL,
    high NUMERIC(20, 8) NOT NULL,
    low NUMERIC(20, 8) NOT NULL,
    close NUMERIC(20, 8) NOT NULL,
    volume NUMERIC(20, 8) NOT NULL,
    PRIMARY KEY (ts, symbol, interval)
);

-- Convert the standard PostgreSQL table to a TimescaleDB hypertable partitioned by the 'ts' column
SELECT create_hypertable('candlesticks', by_range('ts'), if_not_exists => TRUE);

-- Create a composite index for fast querying by symbol, interval, and time (descending for recent data)
CREATE INDEX IF NOT EXISTS ix_candlesticks_symbol_interval ON candlesticks (symbol, interval, ts DESC);
