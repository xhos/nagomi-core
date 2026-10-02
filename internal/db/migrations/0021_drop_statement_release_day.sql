-- +goose Up
-- statements are due as soon as their period ends, so the release day is unused
ALTER TABLE accounts DROP COLUMN statement_release_day;

-- +goose Down
ALTER TABLE accounts ADD COLUMN statement_release_day SMALLINT;
