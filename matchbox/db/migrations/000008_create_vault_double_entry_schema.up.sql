BEGIN;

CREATE TYPE account_type AS ENUM ('ASSET', 'LIABILITY', 'EQUITY', 'REVENUE', 'EXPENSE');
CREATE TYPE account_status AS ENUM ('ACTIVE', 'FROZEN', 'CLOSED');

CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(100) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    type account_type NOT NULL,
    currency VARCHAR(20) NOT NULL,
    status account_status NOT NULL DEFAULT 'ACTIVE',
    balance DECIMAL(32, 18) NOT NULL DEFAULT 0.0,
    CONSTRAINT balance_non_negative CHECK (balance >= 0)
);
CREATE INDEX idx_accounts_currency ON accounts(currency);
CREATE INDEX idx_accounts_code ON accounts(code);

CREATE TABLE journal_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    description TEXT NOT NULL,
    reference_id VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_journal_reference ON journal_entries(reference_id);

CREATE TABLE ledger_postings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    journal_entry_id UUID NOT NULL REFERENCES journal_entries(id) ON DELETE RESTRICT,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    amount DECIMAL(32, 18) NOT NULL,
    direction VARCHAR(6) NOT NULL CHECK (direction IN ('DEBIT', 'CREDIT')),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT amount_positive CHECK (amount > 0)
);
CREATE INDEX idx_ledger_journal ON ledger_postings(journal_entry_id);
CREATE INDEX idx_ledger_account ON ledger_postings(account_id);

CREATE OR REPLACE FUNCTION trg_prevent_mutation() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'CRITICAL: Ledger Postings and Journal Entries are IMMUTABLE. UPDATE and DELETE operations are strictly FORBIDDEN.';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER prevent_ledger_update BEFORE UPDATE ON ledger_postings FOR EACH ROW EXECUTE FUNCTION trg_prevent_mutation();
CREATE TRIGGER prevent_ledger_delete BEFORE DELETE ON ledger_postings FOR EACH ROW EXECUTE FUNCTION trg_prevent_mutation();
CREATE TRIGGER prevent_journal_update BEFORE UPDATE ON journal_entries FOR EACH ROW EXECUTE FUNCTION trg_prevent_mutation();
CREATE TRIGGER prevent_journal_delete BEFORE DELETE ON journal_entries FOR EACH ROW EXECUTE FUNCTION trg_prevent_mutation();

CREATE OR REPLACE FUNCTION trg_update_account_balance() RETURNS TRIGGER AS $$
DECLARE
    acct_type account_type;
    balance_change DECIMAL(32,18);
BEGIN
    SELECT type INTO acct_type FROM accounts WHERE id = NEW.account_id FOR UPDATE;
    IF NEW.direction = 'DEBIT' THEN
        IF acct_type IN ('ASSET', 'EXPENSE') THEN balance_change := NEW.amount;
        ELSE balance_change := -NEW.amount; END IF;
    ELSE
        IF acct_type IN ('ASSET', 'EXPENSE') THEN balance_change := -NEW.amount;
        ELSE balance_change := NEW.amount; END IF;
    END IF;
    UPDATE accounts SET balance = balance + balance_change WHERE id = NEW.account_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER update_balance_after_posting AFTER INSERT ON ledger_postings FOR EACH ROW EXECUTE FUNCTION trg_update_account_balance();

CREATE OR REPLACE FUNCTION trg_check_double_entry() RETURNS TRIGGER AS $$
DECLARE
    debit_total DECIMAL(32,18);
    credit_total DECIMAL(32,18);
BEGIN
    SELECT COALESCE(SUM(CASE WHEN direction = 'DEBIT' THEN amount ELSE 0 END), 0),
           COALESCE(SUM(CASE WHEN direction = 'CREDIT' THEN amount ELSE 0 END), 0)
    INTO debit_total, credit_total
    FROM ledger_postings WHERE journal_entry_id = NEW.journal_entry_id;
    IF debit_total != credit_total THEN
        RAISE EXCEPTION 'DOUBLE-ENTRY EQUILIBRIUM VIOLATION for Journal Entry %: Total DEBIT (%) does not equal Total CREDIT (%)', NEW.journal_entry_id, debit_total, credit_total;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER check_double_entry_balance AFTER INSERT ON ledger_postings DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION trg_check_double_entry();

COMMIT;
