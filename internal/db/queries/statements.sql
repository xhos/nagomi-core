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

-- name: CountExistingExternalIDs :one
select
  count(*)
from
  transactions
where
  account_id = @account_id::bigint
  and external_id = any(@external_ids::text[]);
