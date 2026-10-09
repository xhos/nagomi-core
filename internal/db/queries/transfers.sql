-- name: ListTransferCandidates :many
-- every transaction in the window that could be one side of a transfer
select
  t.id,
  t.account_id,
  t.tx_date,
  t.tx_amount_cents,
  t.tx_currency,
  t.tx_direction,
  t.tx_desc,
  t.merchant,
  t.transfer_ref
from transactions t
join accounts a on a.id = t.account_id
where a.owner_id = @user_id::uuid
  and a.account_type != 6
  and t.split_from_id is null
  and t.tx_date >= @start_date::timestamptz
  and t.tx_date <= @end_date::timestamptz;

-- name: ListTransferAccounts :many
select a.id, a.name, a.bank, a.friendly_name, a.aliases, a.account_type
from accounts a
where a.owner_id = @user_id::uuid
  and a.account_type != 6;

-- name: GetTransactionDateRange :one
select
  min(t.tx_date)::timestamptz as start_date,
  max(t.tx_date)::timestamptz as end_date
from transactions t
join accounts a on a.id = t.account_id
where a.owner_id = @user_id::uuid
  and (sqlc.narg('ids')::bigint[] is null or t.id = any(sqlc.narg('ids')::bigint[]));

-- name: ListTransfersTouching :many
select id, out_tx_id, in_tx_id, status, method
from transfers
where user_id = @user_id::uuid
  and (out_tx_id = any(@ids::bigint[]) or in_tx_id = any(@ids::bigint[]));

-- name: DeleteSuggestedTransfers :exec
delete from transfers
where user_id = @user_id::uuid
  and status = 2
  and (out_tx_id = any(@ids::bigint[]) or in_tx_id = any(@ids::bigint[]));

-- name: CreateTransfer :exec
-- never overrides a pair the user already linked or rejected
insert into transfers (user_id, out_tx_id, in_tx_id, status, method)
values (@user_id::uuid, @out_tx_id::bigint, @in_tx_id::bigint, @status::smallint, @method::smallint)
on conflict (out_tx_id, in_tx_id) do nothing;

-- name: LinkTransferManually :exec
insert into transfers (user_id, out_tx_id, in_tx_id, status, method)
values (@user_id::uuid, @out_tx_id::bigint, @in_tx_id::bigint, 1, 3)
on conflict (out_tx_id, in_tx_id) do update set status = 1, method = 3;

-- name: RejectTransfer :execrows
update transfers
set status = 3
where user_id = @user_id::uuid
  and status = 1
  and (out_tx_id = @tx_id::bigint or in_tx_id = @tx_id::bigint);

-- name: RejectTransferPair :execrows
update transfers
set status = 3
where user_id = @user_id::uuid
  and status in (1, 2)
  and ((out_tx_id = @a::bigint and in_tx_id = @b::bigint) or (out_tx_id = @b::bigint and in_tx_id = @a::bigint));

-- name: CountLinkedTransfers :one
select count(*)
from transfers
where status = 1
  and (out_tx_id = any(@ids::bigint[]) or in_tx_id = any(@ids::bigint[]));

-- name: ListLinkedTransfers :many
-- the linked transfer of each given transaction, with both sides' amounts
select
  tr.id,
  tr.method,
  o.id as out_id,
  o.account_id as out_account_id,
  o.tx_amount_cents as out_amount_cents,
  o.tx_currency as out_currency,
  o.tx_date as out_date,
  coalesce(o.tx_desc, o.merchant) as out_description,
  i.id as in_id,
  i.account_id as in_account_id,
  i.tx_amount_cents as in_amount_cents,
  i.tx_currency as in_currency,
  i.tx_date as in_date,
  coalesce(i.tx_desc, i.merchant) as in_description
from transfers tr
join transactions o on o.id = tr.out_tx_id
join transactions i on i.id = tr.in_tx_id
where tr.status = 1
  and (tr.out_tx_id = any(@ids::bigint[]) or tr.in_tx_id = any(@ids::bigint[]));

-- name: ListTransferSuggestions :many
select sqlc.embed(o), sqlc.embed(i)
from transfers tr
join transactions o on o.id = tr.out_tx_id
join transactions i on i.id = tr.in_tx_id
where tr.user_id = @user_id::uuid
  and tr.status = 2
order by o.tx_date desc, tr.id;

-- name: SetTransferRef :one
-- fills in the reference on a row imported before its importer sent one
update transactions t
set transfer_ref = @transfer_ref::text
from accounts a
where a.id = t.account_id
  and a.owner_id = @user_id::uuid
  and t.account_id = @account_id::bigint
  and t.external_id = @external_id::text
  and t.transfer_ref is null
returning t.id;
