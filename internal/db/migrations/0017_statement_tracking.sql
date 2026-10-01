-- +goose Up
ALTER TABLE accounts
  ADD COLUMN statements_start DATE,
  ADD COLUMN statement_release_day SMALLINT CHECK (statement_release_day BETWEEN 1 AND 31),
  ADD COLUMN closed_at DATE;

-- +goose Down
ALTER TABLE accounts
  DROP COLUMN statements_start,
  DROP COLUMN statement_release_day,
  DROP COLUMN closed_at;
