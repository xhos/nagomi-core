-- +goose Up
-- rule edits used to null is_active; rules treat null as active
UPDATE transaction_rules SET is_active = true WHERE is_active IS NULL;

-- +goose Down
SELECT 1;
