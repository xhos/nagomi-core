-- +goose Up
ALTER TABLE accounts ADD COLUMN statement_driven BOOLEAN NOT NULL DEFAULT FALSE;

-- statements can only be imported into statement-driven accounts from now on,
-- so accounts that already have some keep accepting them
UPDATE accounts SET statement_driven = TRUE
WHERE id IN (SELECT account_id FROM statements WHERE account_id IS NOT NULL);

-- +goose Down
ALTER TABLE accounts DROP COLUMN statement_driven;
