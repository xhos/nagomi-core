-- +goose Up
-- accounts used three colors for gradients; one is enough to tell them apart
ALTER TABLE accounts ADD COLUMN color TEXT CHECK (color ~ '^#[0-9a-fA-F]{6}$');
UPDATE accounts SET color = coalesce(colors[2], '#3b82f6');
ALTER TABLE accounts ALTER COLUMN color SET NOT NULL, DROP COLUMN colors;

-- +goose Down
ALTER TABLE accounts ADD COLUMN colors TEXT[];
UPDATE accounts SET colors = ARRAY[color, color, color];
ALTER TABLE accounts DROP COLUMN color;
