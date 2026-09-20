-- name: ListTransactions :many
select
  sqlc.embed(t),
  COALESCE((SELECT r.id FROM receipts r WHERE r.transaction_id = t.id LIMIT 1), 0)::bigint as receipt_id
from
  transactions t
  join accounts a on t.account_id = a.id
  left join account_users au on a.id = au.account_id
  and au.user_id = sqlc.arg(user_id)::uuid
  left join categories c on t.category_id = c.id
where
  (
    a.owner_id = sqlc.arg(user_id)::uuid
    or au.user_id is not null
  )
  and (
    sqlc.narg('cursor_date')::timestamptz is null
    or sqlc.narg('cursor_id')::bigint is null
    or (t.tx_date, t.id) < (
      sqlc.narg('cursor_date')::timestamptz,
      sqlc.narg('cursor_id')::bigint
    )
  )
  and (
    sqlc.narg('start')::timestamptz is null
    or t.tx_date >= sqlc.narg('start')::timestamptz
  )
  and (
    sqlc.narg('end')::timestamptz is null
    or t.tx_date <= sqlc.narg('end')::timestamptz
  )
  and (
    sqlc.narg('amount_min_cents')::bigint is null
    or t.tx_amount_cents >= sqlc.narg('amount_min_cents')::bigint
  )
  and (
    sqlc.narg('amount_max_cents')::bigint is null
    or t.tx_amount_cents <= sqlc.narg('amount_max_cents')::bigint
  )
  and (
    sqlc.narg('direction')::smallint is null
    or t.tx_direction = sqlc.narg('direction')::smallint
  )
  and (
    sqlc.narg('account_ids')::bigint [] is null
    or t.account_id = ANY(sqlc.narg('account_ids')::bigint [])
  )
  and (
    sqlc.narg('categories')::text [] is null
    or c.slug = ANY(sqlc.narg('categories')::text [])
  )
  and (
    sqlc.narg('merchant_q')::text is null
    or t.merchant ILIKE ('%' || sqlc.narg('merchant_q')::text || '%')
  )
  and (
    sqlc.narg('desc_q')::text is null
    or t.tx_desc ILIKE ('%' || sqlc.narg('desc_q')::text || '%')
  )
  and (
    sqlc.narg('currency')::char(3) is null
    or t.tx_currency = sqlc.narg('currency')::char(3)
  )
  and (
    sqlc.narg('tod_start')::time is null
    or t.tx_date::time >= sqlc.narg('tod_start')::time
  )
  and (
    sqlc.narg('tod_end')::time is null
    or t.tx_date::time <= sqlc.narg('tod_end')::time
  )
  and (
    sqlc.narg('uncategorized')::boolean is null
    or (
      sqlc.narg('uncategorized')::boolean = true
      and t.category_id is null
    )
  )
order by
  t.tx_date desc,
  t.id desc
limit
  COALESCE(sqlc.narg('limit')::int, 100);

-- name: CountTransactions :one
select count(*)
from
  transactions t
  join accounts a on t.account_id = a.id
  left join account_users au on a.id = au.account_id
  and au.user_id = sqlc.arg(user_id)::uuid
  left join categories c on t.category_id = c.id
where
  (
    a.owner_id = sqlc.arg(user_id)::uuid
    or au.user_id is not null
  )
  and (
    sqlc.narg('start')::timestamptz is null
    or t.tx_date >= sqlc.narg('start')::timestamptz
  )
  and (
    sqlc.narg('end')::timestamptz is null
    or t.tx_date <= sqlc.narg('end')::timestamptz
  )
  and (
    sqlc.narg('amount_min_cents')::bigint is null
    or t.tx_amount_cents >= sqlc.narg('amount_min_cents')::bigint
  )
  and (
    sqlc.narg('amount_max_cents')::bigint is null
    or t.tx_amount_cents <= sqlc.narg('amount_max_cents')::bigint
  )
  and (
    sqlc.narg('direction')::smallint is null
    or t.tx_direction = sqlc.narg('direction')::smallint
  )
  and (
    sqlc.narg('account_ids')::bigint [] is null
    or t.account_id = ANY(sqlc.narg('account_ids')::bigint [])
  )
  and (
    sqlc.narg('categories')::text [] is null
    or c.slug = ANY(sqlc.narg('categories')::text [])
  )
  and (
    sqlc.narg('merchant_q')::text is null
    or t.merchant ILIKE ('%' || sqlc.narg('merchant_q')::text || '%')
  )
  and (
    sqlc.narg('desc_q')::text is null
    or t.tx_desc ILIKE ('%' || sqlc.narg('desc_q')::text || '%')
  )
  and (
    sqlc.narg('currency')::char(3) is null
    or t.tx_currency = sqlc.narg('currency')::char(3)
  )
  and (
    sqlc.narg('tod_start')::time is null
    or t.tx_date::time >= sqlc.narg('tod_start')::time
  )
  and (
    sqlc.narg('tod_end')::time is null
    or t.tx_date::time <= sqlc.narg('tod_end')::time
  )
  and (
    sqlc.narg('uncategorized')::boolean is null
    or (
      sqlc.narg('uncategorized')::boolean = true
      and t.category_id is null
    )
  );

-- name: GetTransaction :one
select
  t.*
from
  transactions t
  join accounts a on t.account_id = a.id
  left join account_users au on a.id = au.account_id
  and au.user_id = sqlc.arg(user_id)::uuid
where
  t.id = sqlc.arg(id)::bigint
  and (
    a.owner_id = sqlc.arg(user_id)::uuid
    or au.user_id is not null
  );

-- name: CreateTransaction :one
insert into
  transactions (
    external_id,
    account_id,
    tx_date,
    tx_amount_cents,
    tx_currency,
    tx_direction,
    tx_desc,
    balance_after_cents,
    balance_currency,
    category_id,
    category_manually_set,
    merchant,
    merchant_manually_set,
    user_notes,
    foreign_amount_cents,
    foreign_currency,
    exchange_rate,
    suggestions,
    split_from_id,
    forgiven
  )
select
  sqlc.narg('external_id')::text,
  sqlc.arg(account_id)::bigint,
  sqlc.arg(tx_date)::timestamptz,
  sqlc.arg(tx_amount_cents)::bigint,
  sqlc.arg(tx_currency)::char(3),
  sqlc.arg(tx_direction)::smallint,
  sqlc.narg('tx_desc')::text,
  sqlc.narg('balance_after_cents')::bigint,
  sqlc.narg('balance_currency')::char(3),
  sqlc.narg('category_id')::bigint,
  sqlc.narg('category_manually_set')::boolean,
  sqlc.narg('merchant')::text,
  sqlc.narg('merchant_manually_set')::boolean,
  sqlc.narg('user_notes')::text,
  sqlc.narg('foreign_amount_cents')::bigint,
  sqlc.narg('foreign_currency')::char(3),
  sqlc.narg('exchange_rate')::double precision,
  sqlc.narg('suggestions')::text [],
  sqlc.narg('split_from_id')::bigint,
  coalesce(sqlc.narg('forgiven')::boolean, false)
from
  accounts a
  left join account_users au on a.id = au.account_id
  and au.user_id = sqlc.arg(user_id)::uuid
where
  a.id = sqlc.arg(account_id)::bigint
  and (
    a.owner_id = sqlc.arg(user_id)::uuid
    or au.user_id is not null
  )
on conflict (account_id, external_id) where external_id is not null do nothing
returning
  *;

-- name: BulkCreateTransactions :many
insert into
  transactions (
    account_id,
    tx_date,
    tx_amount_cents,
    tx_currency,
    tx_direction,
    tx_desc,
    category_id,
    merchant,
    user_notes,
    foreign_amount_cents,
    foreign_currency,
    exchange_rate
  )
select
  unnest(@account_ids::bigint[]),
  unnest(@tx_dates::timestamptz[]),
  unnest(@tx_amount_cents::bigint[]),
  unnest(@tx_currencies::char(3)[]),
  unnest(@tx_directions::smallint[]),
  unnest(@tx_descs::text[]),
  unnest(@category_ids::bigint[]),
  unnest(@merchants::text[]),
  unnest(@user_notes::text[]),
  unnest(@foreign_amount_cents::bigint[]),
  unnest(@foreign_currencies::char(3)[]),
  unnest(@exchange_rates::double precision[])
returning
  *;

-- name: UpdateTransaction :exec
update
  transactions
set
  external_id = coalesce(sqlc.narg('external_id')::text, external_id),
  account_id = coalesce(sqlc.narg('account_id')::bigint, account_id),
  tx_date = coalesce(sqlc.narg('tx_date')::timestamptz, tx_date),
  tx_amount_cents = coalesce(sqlc.narg('tx_amount_cents')::bigint, tx_amount_cents),
  tx_currency = coalesce(sqlc.narg('tx_currency')::char(3), tx_currency),
  tx_direction = coalesce(sqlc.narg('tx_direction')::smallint, tx_direction),
  tx_desc = coalesce(sqlc.narg('tx_desc')::text, tx_desc),
  category_id = case when @clear_category::boolean then null else coalesce(sqlc.narg('category_id')::bigint, category_id) end,
  merchant = coalesce(sqlc.narg('merchant')::text, merchant),
  user_notes = coalesce(sqlc.narg('user_notes')::text, user_notes),
  foreign_amount_cents = coalesce(sqlc.narg('foreign_amount_cents')::bigint, foreign_amount_cents),
  foreign_currency = coalesce(sqlc.narg('foreign_currency')::char(3), foreign_currency),
  exchange_rate = coalesce(sqlc.narg('exchange_rate')::double precision, exchange_rate),
  suggestions = coalesce(sqlc.narg('suggestions')::text[], suggestions),
  category_manually_set = coalesce(sqlc.narg('category_manually_set')::boolean, category_manually_set),
  merchant_manually_set = coalesce(sqlc.narg('merchant_manually_set')::boolean, merchant_manually_set),
  forgiven = coalesce(sqlc.narg('forgiven')::boolean, forgiven)
where
  id = sqlc.arg(id)::bigint
  and account_id in (
    select
      a.id
    from
      accounts a
      left join account_users au on a.id = au.account_id
      and au.user_id = sqlc.arg(user_id)::uuid
    where
      a.owner_id = sqlc.arg(user_id)::uuid
      or au.user_id is not null
  );

-- name: DeleteTransaction :execrows
delete from
  transactions
where
  id = sqlc.arg(id)::bigint
  and account_id in (
    select
      a.id
    from
      accounts a
      left join account_users au on a.id = au.account_id
      and au.user_id = sqlc.arg(user_id)::uuid
    where
      a.owner_id = sqlc.arg(user_id)::uuid
      or au.user_id is not null
  );

-- name: CategorizeTransactionAtomic :one
update
  transactions
set
  category_id = sqlc.narg('category_id')::bigint,
  category_manually_set = sqlc.arg(category_manually_set)::boolean,
  suggestions = sqlc.arg(suggestions)::text []
where
  id = sqlc.arg(id)::bigint
  and category_manually_set = false
  and account_id in (
    select
      a.id
    from
      accounts a
      left join account_users au on a.id = au.account_id
      and au.user_id = sqlc.arg(user_id)::uuid
    where
      a.owner_id = sqlc.arg(user_id)::uuid
      or au.user_id is not null
  )
returning
  id,
  category_manually_set;

-- name: BulkCategorizeTransactions :execrows
update
  transactions
set
  category_id = sqlc.arg(category_id)::bigint,
  category_manually_set = true
where
  id = ANY(sqlc.arg(transaction_ids)::bigint [])
  and account_id in (
    select
      a.id
    from
      accounts a
      left join account_users au on a.id = au.account_id
      and au.user_id = sqlc.arg(user_id)::uuid
    where
      a.owner_id = sqlc.arg(user_id)::uuid
      or au.user_id is not null
  );

-- name: BulkDeleteTransactions :execrows
delete from
  transactions
where
  id = ANY(sqlc.arg(transaction_ids)::bigint [])
  and account_id in (
    select
      a.id
    from
      accounts a
      left join account_users au on a.id = au.account_id
      and au.user_id = sqlc.arg(user_id)::uuid
    where
      a.owner_id = sqlc.arg(user_id)::uuid
      or au.user_id is not null
  );

-- name: GetTransactionCountByAccount :many
select
  a.id,
  a.name,
  COUNT(t.id) as transaction_count
from
  accounts a
  left join account_users au on a.id = au.account_id
  and au.user_id = sqlc.arg(user_id)::uuid
  left join transactions t on a.id = t.account_id
where
  (a.owner_id = sqlc.arg(user_id)::uuid or au.user_id is not null)
  and a.account_type != 6
group by
  a.id,
  a.name
order by
  transaction_count desc;

-- name: FindCandidateTransactions :many
select
  sqlc.embed(t),
  similarity(t.tx_desc::text, sqlc.arg(merchant)::text) as merchant_score
from
  transactions t
  join accounts a on t.account_id = a.id
  left join account_users au on a.id = au.account_id
  and au.user_id = sqlc.arg(user_id)::uuid
where
  (
    a.owner_id = sqlc.arg(user_id)::uuid
    or au.user_id is not null
  )
  and t.tx_direction = 2
  and t.tx_date >= (sqlc.arg(date)::date - interval '60 days')
  and t.tx_amount_cents between sqlc.arg(total_cents)::bigint and (sqlc.arg(total_cents)::bigint * 120 / 100)
  and similarity(t.tx_desc::text, sqlc.arg(merchant)::text) > 0.3
order by
  merchant_score desc
limit
  10;

-- name: GetAccountIDsFromTransactionIDs :many
select
  distinct account_id
from
  transactions
where
  id = ANY(@ids::bigint []);

-- name: ListAllTransactions :many
select
  t.*
from
  transactions t
  join accounts a on t.account_id = a.id
  left join account_users au on a.id = au.account_id
  and au.user_id = sqlc.arg(user_id)::uuid
where
  (
    a.owner_id = sqlc.arg(user_id)::uuid
    or au.user_id is not null
  )
order by
  t.tx_date desc,
  t.id desc;

-- name: GetSplitsBySourceID :many
select t.*
from transactions t
where t.split_from_id = @source_id::bigint
order by t.id;

-- name: UpdateTransactionForgiven :exec
update transactions
set forgiven = @forgiven::boolean
where id = @id::bigint
  and account_id in (
    select a.id
    from accounts a
    left join account_users au on a.id = au.account_id and au.user_id = @user_id::uuid
    where a.owner_id = @user_id::uuid or au.user_id is not null
  );

-- name: GetFriendAccountIDsFromSplits :many
select distinct t.account_id
from transactions t
where t.split_from_id = any(@ids::bigint[]);
