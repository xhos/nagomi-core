-- +goose Up
-- protovalidate enforces these at the api boundary
ALTER TABLE accounts DROP CONSTRAINT accounts_color_check, DROP CONSTRAINT accounts_statement_release_day_check;

-- +goose Down
ALTER TABLE accounts
  ADD CONSTRAINT accounts_color_check CHECK (color ~ '^#[0-9a-fA-F]{6}$'),
  ADD CONSTRAINT accounts_statement_release_day_check CHECK (statement_release_day BETWEEN 1 AND 31);
