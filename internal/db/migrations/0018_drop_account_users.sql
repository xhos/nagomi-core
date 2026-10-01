-- +goose Up
-- account sharing was never built; every account has only its owner
DROP TABLE account_users;

-- +goose Down
CREATE TABLE account_users (
  account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  user_id    UUID   NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  added_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (account_id, user_id)
);

CREATE INDEX idx_account_users_user_account ON account_users(user_id, account_id);
