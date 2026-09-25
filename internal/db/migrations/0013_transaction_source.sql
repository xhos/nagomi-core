-- +goose Up
-- values mirror nagomi.v1.TransactionSource
ALTER TABLE transactions ADD COLUMN source SMALLINT NOT NULL DEFAULT 1;

-- connector rows can't be told apart from email ones here; both are
-- provisional, so tagging them all as email is good enough
UPDATE transactions SET source = CASE
  WHEN split_from_id IS NOT NULL THEN 5
  WHEN external_id IS NOT NULL THEN 2
  ELSE 1
END;

-- +goose Down
ALTER TABLE transactions DROP COLUMN source;
