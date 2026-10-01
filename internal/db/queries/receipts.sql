-- name: CreateReceipt :one
INSERT INTO receipts (
  user_id,
  image_path,
  image_hash,
  image_taken_at,
  status
)
VALUES (
  sqlc.arg(user_id)::uuid,
  sqlc.arg(image_path)::text,
  sqlc.arg(image_hash)::text,
  sqlc.narg('image_taken_at')::timestamptz,
  sqlc.arg(status)::smallint
)
RETURNING *;

-- name: CreateReceiptRecord :one
INSERT INTO receipts (
  user_id,
  transaction_id,
  merchant,
  receipt_date,
  currency,
  subtotal_cents,
  tax_cents,
  total_cents,
  status,
  source
)
VALUES (
  sqlc.arg(user_id)::uuid,
  sqlc.narg('transaction_id')::bigint,
  sqlc.narg('merchant')::text,
  sqlc.narg('receipt_date')::date,
  sqlc.narg('currency')::text,
  sqlc.narg('subtotal_cents')::bigint,
  sqlc.narg('tax_cents')::bigint,
  sqlc.narg('total_cents')::bigint,
  sqlc.arg(status)::smallint,
  'manual'
)
RETURNING *;

-- name: GetReceiptByImageHash :one
SELECT *
FROM receipts
WHERE user_id = sqlc.arg(user_id)::uuid
  AND image_hash = sqlc.arg(image_hash)::text;

-- name: GetReceipt :one
SELECT *
FROM receipts
WHERE id = sqlc.arg(id)::bigint
  AND user_id = sqlc.arg(user_id)::uuid;

-- name: ListReceipts :many
SELECT
  r.*,
  count(*) OVER() AS total_count
FROM receipts r
WHERE r.user_id = sqlc.arg(user_id)::uuid
  AND (
    sqlc.narg('status')::smallint IS NULL
    OR r.status = sqlc.narg('status')::smallint
  )
  AND (
    sqlc.narg('unlinked_only')::boolean IS NULL
    OR (sqlc.narg('unlinked_only')::boolean = true AND r.transaction_id IS NULL)
  )
  AND (
    sqlc.narg('start_date')::date IS NULL
    OR r.receipt_date >= sqlc.narg('start_date')::date
  )
  AND (
    sqlc.narg('end_date')::date IS NULL
    OR r.receipt_date <= sqlc.narg('end_date')::date
  )
  AND (
    sqlc.narg('query')::text IS NULL
    OR r.merchant ILIKE '%' || sqlc.narg('query')::text || '%'
    OR EXISTS (
      SELECT 1 FROM receipt_items ri
      WHERE ri.receipt_id = r.id
        AND (
          ri.name ILIKE '%' || sqlc.narg('query')::text || '%'
          OR ri.raw_name ILIKE '%' || sqlc.narg('query')::text || '%'
        )
    )
  )
  AND (
    sqlc.narg('min_total_cents')::bigint IS NULL
    OR r.total_cents >= sqlc.narg('min_total_cents')::bigint
  )
  AND (
    sqlc.narg('max_total_cents')::bigint IS NULL
    OR r.total_cents <= sqlc.narg('max_total_cents')::bigint
  )
  AND (
    sqlc.narg('currency')::char(3) IS NULL
    OR r.currency = sqlc.narg('currency')::char(3)
  )
ORDER BY r.created_at DESC, r.id DESC
LIMIT COALESCE(sqlc.narg('lim')::int, 50)
OFFSET COALESCE(sqlc.narg('off')::int, 0);

-- name: UpdateReceipt :one
UPDATE receipts
SET
  transaction_id = coalesce(sqlc.narg('transaction_id')::bigint, transaction_id),
  merchant       = coalesce(sqlc.narg('merchant')::text, merchant),
  receipt_date   = coalesce(sqlc.narg('receipt_date')::date, receipt_date),
  currency       = coalesce(sqlc.narg('currency')::char(3), currency),
  subtotal_cents = coalesce(sqlc.narg('subtotal_cents')::bigint, subtotal_cents),
  tax_cents      = coalesce(sqlc.narg('tax_cents')::bigint, tax_cents),
  total_cents    = coalesce(sqlc.narg('total_cents')::bigint, total_cents),
  confidence     = coalesce(sqlc.narg('confidence')::real, confidence),
  status         = coalesce(sqlc.narg('status')::smallint, status)
WHERE id = sqlc.arg(id)::bigint
  AND user_id = sqlc.arg(user_id)::uuid
RETURNING *;

-- name: ResetReceiptForRetry :one
UPDATE receipts
SET
  status         = 1,
  merchant       = NULL,
  receipt_date   = NULL,
  currency       = NULL,
  subtotal_cents = NULL,
  tax_cents      = NULL,
  total_cents    = NULL,
  confidence     = NULL,
  transaction_id = NULL
WHERE id = sqlc.arg(id)::bigint
  AND user_id = sqlc.arg(user_id)::uuid
RETURNING *;

-- name: DeleteReceipt :exec
DELETE FROM receipts
WHERE id = sqlc.arg(id)::bigint
  AND user_id = sqlc.arg(user_id)::uuid;

-- name: GetPendingReceipts :many
SELECT *
FROM receipts
WHERE status = 1
ORDER BY created_at ASC
LIMIT 20;

-- name: GetParsedUnlinkedReceipts :many
SELECT *
FROM receipts
WHERE status = 2
  AND transaction_id IS NULL
  AND total_cents IS NOT NULL
  AND currency IS NOT NULL
ORDER BY created_at ASC
LIMIT 20;

-- name: CreateReceiptItem :one
INSERT INTO receipt_items (
  receipt_id,
  raw_name,
  name,
  quantity,
  unit_price_cents,
  unit_currency,
  sort_order
)
VALUES (
  sqlc.arg(receipt_id)::bigint,
  sqlc.arg(raw_name)::text,
  sqlc.narg('name')::text,
  sqlc.arg(quantity)::double precision,
  sqlc.arg(unit_price_cents)::bigint,
  sqlc.arg(unit_currency)::char(3),
  sqlc.arg(sort_order)::int
)
RETURNING *;

-- name: ListReceiptItems :many
SELECT *
FROM receipt_items
WHERE receipt_id = sqlc.arg(receipt_id)::bigint
ORDER BY sort_order ASC, id ASC;

-- name: DeleteReceiptItemsByReceipt :exec
DELETE FROM receipt_items
WHERE receipt_id = sqlc.arg(receipt_id)::bigint;

-- name: FindReceiptLinkCandidates :many
SELECT
  t.id,
  t.account_id,
  t.tx_date,
  t.tx_amount_cents,
  t.tx_currency,
  t.merchant,
  COALESCE(a.friendly_name, a.name) AS account_display_name,
  CASE
    WHEN sqlc.narg('best_date')::date IS NOT NULL
    THEN ABS(t.tx_date::date - sqlc.narg('best_date')::date)
    ELSE NULL
  END AS date_diff_days
FROM transactions t
JOIN accounts a ON t.account_id = a.id
WHERE a.owner_id = sqlc.arg(user_id)::uuid
  AND t.tx_amount_cents BETWEEN sqlc.arg(amount_cents)::bigint - 5 AND sqlc.arg(amount_cents)::bigint + 5
  AND t.tx_currency = sqlc.arg(currency)::char(3)
  AND t.tx_direction = 2::smallint
  AND NOT EXISTS (
    SELECT 1 FROM receipts r
    WHERE r.transaction_id = t.id
  )
ORDER BY
  CASE WHEN t.tx_amount_cents = sqlc.arg(amount_cents)::bigint THEN 0 ELSE 1 END ASC,
  CASE
    WHEN sqlc.narg('best_date')::date IS NOT NULL
    THEN ABS(t.tx_date::date - sqlc.narg('best_date')::date)
    ELSE 0
  END ASC,
  t.tx_date DESC
LIMIT 10;
