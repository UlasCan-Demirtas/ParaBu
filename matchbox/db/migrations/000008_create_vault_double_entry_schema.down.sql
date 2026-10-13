BEGIN;

DROP TABLE IF EXISTS ledger_postings;
DROP TABLE IF EXISTS journal_entries;
DROP TABLE IF EXISTS accounts;

DROP TYPE IF EXISTS account_type;
DROP TYPE IF EXISTS account_status;

COMMIT;
