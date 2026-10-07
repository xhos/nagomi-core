-- +goose Up
-- the last few emails per user, trimmed on insert; bodies only for unimported ones
CREATE TABLE received_emails (
  id             BIGSERIAL PRIMARY KEY,
  user_id        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  received_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  sender         TEXT        NOT NULL,
  subject        TEXT        NOT NULL,
  outcome        SMALLINT    NOT NULL,
  transaction_id BIGINT      REFERENCES transactions(id) ON DELETE SET NULL,
  error          TEXT,
  body           TEXT
);

CREATE INDEX received_emails_user_idx ON received_emails (user_id, received_at DESC);

-- +goose Down
DROP TABLE received_emails;
