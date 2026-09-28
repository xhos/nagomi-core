-- +goose Up
-- null when the statement's balances couldn't be read
ALTER TABLE statements ADD COLUMN balance_ok BOOLEAN;

-- +goose Down
ALTER TABLE statements DROP COLUMN balance_ok;
