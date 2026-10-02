-- name: CreateStatement :one
insert into
  statements (
    user_id,
    status,
    file_path,
    file_hash,
    file_name,
    parser,
    bank,
    account_type,
    account_number,
    period_start,
    period_end,
    currency,
    opening_balance_cents,
    closing_balance_cents,
    balance_ok,
    line_count,
    parsed
  )
values
  (
    @user_id::uuid,
    @status::smallint,
    @file_path::text,
    @file_hash::text,
    @file_name::text,
    @parser::text,
    @bank::text,
    @account_type::smallint,
    @account_number::text,
    @period_start::date,
    @period_end::date,
    @currency::char(3),
    sqlc.narg('opening_balance_cents')::bigint,
    sqlc.narg('closing_balance_cents')::bigint,
    sqlc.narg('balance_ok')::boolean,
    @line_count::int,
    @parsed::jsonb
  )
returning
  *;

-- name: GetStatementByHash :one
select
  *
from
  statements
where
  user_id = @user_id::uuid
  and file_hash = @file_hash::text;

-- name: GetStatement :one
select
  sqlc.embed(s),
  a.name as account_name
from
  statements s
  left join accounts a on a.id = s.account_id
where
  s.id = @id::bigint
  and s.user_id = @user_id::uuid;

-- name: ListStatements :many
select
  sqlc.embed(s),
  a.name as account_name
from
  statements s
  left join accounts a on a.id = s.account_id
where
  s.user_id = @user_id::uuid
  and (
    sqlc.narg('account_id')::bigint is null
    or s.account_id = sqlc.narg('account_id')::bigint
  )
  and (
    sqlc.narg('status')::smallint is null
    or s.status = sqlc.narg('status')::smallint
  )
order by
  s.period_start desc,
  s.id desc;

-- name: MarkStatementImported :one
update statements
set
  status = 2,
  account_id = @account_id::bigint,
  imported_at = now()
where
  id = @id::bigint
  and user_id = @user_id::uuid
  and status = 1
returning
  *;

-- name: UpdateStatementParse :one
update statements
set
  parser = @parser::text,
  bank = @bank::text,
  account_type = @account_type::smallint,
  account_number = @account_number::text,
  period_start = @period_start::date,
  period_end = @period_end::date,
  currency = @currency::char(3),
  opening_balance_cents = sqlc.narg('opening_balance_cents')::bigint,
  closing_balance_cents = sqlc.narg('closing_balance_cents')::bigint,
  balance_ok = sqlc.narg('balance_ok')::boolean,
  line_count = @line_count::int,
  parsed = @parsed::jsonb
where
  id = @id::bigint
  and user_id = @user_id::uuid
returning
  *;

-- name: DeleteStatementTransactions :execrows
delete from transactions t
using statements s
where
  t.statement_id = s.id
  and s.id = @id::bigint
  and s.user_id = @user_id::uuid;

-- name: DeleteStatement :one
delete from statements
where
  id = @id::bigint
  and user_id = @user_id::uuid
returning
  file_path;

-- name: DeleteStalePendingStatements :many
delete from statements
where
  status = 1
  and created_at < @before::timestamptz
returning
  file_path;

-- name: ListExistingExternalIDs :many
select
  external_id::text
from
  transactions
where
  account_id = @account_id::bigint
  and external_id = any(@external_ids::text[]);

-- name: ListReconcileCandidates :many
select
  sqlc.embed(t),
  (
    coalesce(t.user_notes, '') <> ''
    or exists(select 1 from receipts r where r.transaction_id = t.id)
    or exists(select 1 from transactions s where s.split_from_id = t.id)
  )::boolean as has_user_data,
  exists(select 1 from transactions s where s.split_from_id = t.id)::boolean as has_splits,
  (t.statement_id is not null)::boolean as from_statement
from
  transactions t
where
  t.account_id = @account_id::bigint
  and t.split_from_id is null
  and (
    (
      t.statement_id is null
      and t.tx_date >= @from_date::timestamptz
      and t.tx_date < @to_date::timestamptz
    )
    -- when re-parsing, the statement's own transactions, whatever their date
    or t.statement_id = sqlc.narg('statement_id')::bigint
  )
order by
  t.tx_date,
  t.id;

-- name: ConfirmStatementTransaction :exec
update transactions
set
  tx_date = @tx_date::timestamptz,
  tx_amount_cents = @tx_amount_cents::bigint,
  tx_desc = @tx_desc::text,
  exchange_rate = coalesce(sqlc.narg('exchange_rate')::double precision, exchange_rate),
  external_id = @external_id::text,
  statement_id = @statement_id::bigint,
  source = @source::smallint
where
  id = @id::bigint
  and account_id = @account_id::bigint;

-- name: LinkStatementTransactions :exec
-- lines kept from a statement that was deleted without its transactions
update transactions
set statement_id = @statement_id::bigint
where
  account_id = @account_id::bigint
  and external_id = any(@external_ids::text[])
  and statement_id is null;

-- name: ListStatementDrivenAccounts :many
select
  id,
  name,
  statements_start,
  closed_at
from
  accounts
where
  owner_id = @user_id::uuid
  and statement_driven
order by
  name,
  id;

-- name: ListImportedStatementPeriods :many
select
  id,
  account_id::bigint,
  period_start,
  period_end,
  balance_ok
from
  statements
where
  user_id = @user_id::uuid
  and status = 2
  and account_id = any(@account_ids::bigint[])
order by
  account_id,
  period_start,
  id;
