-- name: ListAccounts :many
select
  sqlc.embed(a),
  COALESCE(
    (select t.balance_after_cents
     from transactions t
     where t.account_id = a.id
     order by t.tx_date desc, t.id desc
     limit 1),
    a.anchor_balance_cents
  ) as balance_cents,
  COALESCE(
    (select t.balance_currency
     from transactions t
     where t.account_id = a.id
     order by t.tx_date desc, t.id desc
     limit 1),
    a.anchor_currency
  ) as balance_currency
from
  accounts a
where
  a.owner_id = @user_id::uuid
order by
  a.created_at;

-- name: GetAccount :one
select
  sqlc.embed(a),
  COALESCE(
    (select t.balance_after_cents
     from transactions t
     where t.account_id = a.id
     order by t.tx_date desc, t.id desc
     limit 1),
    a.anchor_balance_cents
  ) as balance_cents,
  COALESCE(
    (select t.balance_currency
     from transactions t
     where t.account_id = a.id
     order by t.tx_date desc, t.id desc
     limit 1),
    a.anchor_currency
  ) as balance_currency
from
  accounts a
where
  a.id = @id::bigint
  and a.owner_id = @user_id::uuid;

-- name: CreateAccount :one
insert into
  accounts (
    owner_id,
    name,
    bank,
    account_type,
    friendly_name,
    anchor_balance_cents,
    anchor_currency,
    main_currency,
    color,
    statement_driven
  )
values
  (
    @owner_id::uuid,
    @name::text,
    @bank::text,
    @account_type::smallint,
    sqlc.narg('friendly_name')::text,
    @anchor_balance_cents::bigint,
    @anchor_currency::char(3),
    @main_currency::char(3),
    coalesce(sqlc.narg('color')::text, '#3b82f6'),
    @statement_driven::boolean
  )
returning
  *;

-- name: UpdateAccount :exec
update
  accounts
set
  name = coalesce(sqlc.narg('name')::text, name),
  bank = coalesce(sqlc.narg('bank')::text, bank),
  account_type = coalesce(sqlc.narg('account_type')::smallint, account_type),
  friendly_name = coalesce(sqlc.narg('friendly_name')::text, friendly_name),
  anchor_date = coalesce(sqlc.narg('anchor_date')::date, anchor_date),
  anchor_balance_cents = coalesce(sqlc.narg('anchor_balance_cents')::bigint, anchor_balance_cents),
  anchor_currency = coalesce(sqlc.narg('anchor_currency')::char(3), anchor_currency),
  main_currency = coalesce(sqlc.narg('main_currency')::char(3), main_currency),
  color = coalesce(sqlc.narg('color')::text, color),
  statement_driven = coalesce(sqlc.narg('statement_driven')::boolean, statement_driven),
  statements_start = case
    when @clear_statements_start::boolean then null
    else coalesce(sqlc.narg('statements_start')::date, statements_start)
  end,
  closed_at = case
    when @clear_closed_at::boolean then null
    else coalesce(sqlc.narg('closed_at')::date, closed_at)
  end
where
  id = @id::bigint
  and owner_id = @user_id::uuid;

-- name: DeleteAccount :execrows
delete from
  accounts
where
  id = @id::bigint
  and owner_id = @user_id::uuid;

-- name: SetAccountAnchor :execrows
update
  accounts
set
  anchor_date = now()::date,
  anchor_balance_cents = @anchor_balance_cents::bigint,
  anchor_currency = @anchor_currency::char(3)
where
  id = @id::bigint
  and owner_id = @user_id::uuid;

-- name: GetAccountAnchorBalance :one
select
  anchor_balance_cents,
  anchor_currency
from
  accounts
where
  id = @id::bigint;

-- name: GetAccountBalance :one
select
  balance_after_cents,
  balance_currency
from
  transactions
where
  account_id = @account_id::bigint
order by
  tx_date desc,
  id desc
limit
  1;

-- name: AccountHasTransactions :one
select exists(
  select 1 from transactions where account_id = @account_id::bigint
) as has_transactions;

-- name: GetUserAccountsCount :one
select
  COUNT(*) as account_count
from
  accounts a
where
  a.owner_id = @user_id::uuid
  and a.account_type != 6;

-- name: AddAccountAlias :exec
update accounts
set aliases = array_append(aliases, @alias::text)
where id = @id::bigint
  and owner_id = @user_id::uuid
  and not (aliases @> array[@alias::text]);

-- name: RemoveAccountAlias :exec
update accounts
set aliases = array_remove(aliases, @alias::text)
where id = @id::bigint
  and owner_id = @user_id::uuid;

-- name: SetAccountAliases :exec
update accounts
set aliases = @aliases::text[]
where id = @id::bigint
  and owner_id = @user_id::uuid;

-- name: FindAccountByAlias :one
select sqlc.embed(a)
from accounts a
where a.owner_id = @user_id::uuid
  and a.aliases @> array[@alias::text]
limit 1;

-- name: FindAccountByName :one
select sqlc.embed(a)
from accounts a
where a.owner_id = @user_id::uuid
  and a.name = @name::text
limit 1;

-- name: MoveAccountTransactions :execrows
update transactions
set account_id = @primary_id::bigint
where account_id = @secondary_id::bigint;

-- name: MoveAccountStatements :execrows
update statements
set account_id = @primary_id::bigint
where account_id = @secondary_id::bigint;

-- name: SyncAccountBalances :exec
with anchor_transactions as (
  select
    t.id,
    t.account_id,
    t.tx_date,
    t.tx_direction,
    t.tx_amount_cents,
    a.main_currency,
    a.anchor_date,
    a.anchor_balance_cents,
    case when t.tx_date >= a.anchor_date then 1 else 0 end as is_after_anchor
  from
    transactions t
    join accounts a on t.account_id = a.id
  where
    t.account_id = @account_id::bigint
    and not t.forgiven
),
before_anchor as (
  select
    id,
    main_currency,
    anchor_balance_cents - COALESCE(sum(
      case
        when tx_direction = 1 then tx_amount_cents
        when tx_direction = 2 then -tx_amount_cents
        else 0
      end
    ) over (
      order by tx_date desc, id desc
      ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING
    ), 0) as balance_after_cents
  from
    anchor_transactions
  where
    is_after_anchor = 0
),
after_anchor as (
  select
    id,
    main_currency,
    anchor_balance_cents + sum(
      case
        when tx_direction = 1 then tx_amount_cents
        when tx_direction = 2 then -tx_amount_cents
        else 0
      end
    ) over (
      order by tx_date, id
    ) as balance_after_cents
  from
    anchor_transactions
  where
    is_after_anchor = 1
)
update
  transactions
set
  balance_after_cents = coalesce(ba.balance_after_cents, aa.balance_after_cents),
  balance_currency = coalesce(ba.main_currency, aa.main_currency)
from
  before_anchor ba
  full outer join after_anchor aa on ba.id = aa.id
where
  transactions.id = coalesce(ba.id, aa.id);

-- name: GetFriendAccountBalances :many
select
  a.id,
  a.name,
  a.anchor_currency as currency,
  COALESCE(
    (select t.balance_after_cents
     from transactions t
     where t.account_id = a.id and not t.forgiven
     order by t.tx_date desc, t.id desc
     limit 1),
    a.anchor_balance_cents
  ) as balance_cents
from accounts a
where a.owner_id = @user_id::uuid
  and a.account_type = 6
order by a.name;
