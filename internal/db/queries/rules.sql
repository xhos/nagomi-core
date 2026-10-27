-- name: ListRules :many
select *
from transaction_rules
where user_id = @user_id::uuid
order by priority_order, created_at;

-- name: GetRule :one
select *
from transaction_rules
where rule_id = @rule_id::uuid
  and user_id = @user_id::uuid;

-- name: CreateRule :one
insert into transaction_rules (user_id, rule_name, category_id, conditions, merchant)
values (@user_id::uuid, @rule_name::text, @category_id::bigint, @conditions::jsonb, @merchant::text)
returning *;

-- name: UpdateRule :exec
update transaction_rules
set
  rule_name = coalesce(sqlc.narg('rule_name'), rule_name),
  category_id = sqlc.narg('category_id')::bigint,
  conditions = coalesce(sqlc.narg('conditions'), conditions),
  is_active = coalesce(sqlc.narg('is_active'), is_active),
  priority_order = coalesce(sqlc.narg('priority_order'), priority_order),
  merchant = sqlc.narg('merchant'),
  updated_at = now()
where rule_id = @rule_id::uuid
  and user_id = @user_id::uuid;

-- name: DeleteRule :execrows
delete from transaction_rules
where rule_id = @rule_id::uuid
  and user_id = @user_id::uuid;

-- name: GetActiveRules :many
select *
from transaction_rules
where user_id = @user_id::uuid
  and (is_active is null or is_active = true)
order by priority_order, created_at;

-- name: GetTransactionsForRuleApplication :many
select
  t.*
from transactions t
join accounts a on t.account_id = a.id
where a.owner_id = @user_id::uuid
  and (sqlc.narg('transaction_ids')::bigint[] is null or t.id = ANY(sqlc.narg('transaction_ids')::bigint[]))
  -- BulkApplyRuleToTransactions guards each field, so a hand-set merchant still lets a rule set the category
  and (sqlc.narg('include_manually_set')::boolean = true or t.category_manually_set = false or t.merchant_manually_set = false);

-- name: BulkApplyRuleToTransactions :execrows
update transactions
set
  category_id = case
    when @category_id::bigint > 0 and category_manually_set = false
    then @category_id::bigint
    else category_id
  end,
  merchant = case
    when @merchant::text != '' and merchant_manually_set = false
    then @merchant::text
    else merchant
  end
where id = ANY(@transaction_ids::bigint[])
  and account_id in (
    select a.id
    from accounts a
    where a.owner_id = @user_id::uuid
  )
  and (
    (@category_id::bigint > 0 and category_manually_set = false) or
    (@merchant::text != '' and merchant_manually_set = false)
  );
-- name: ListUncategorizedDescriptions :many
-- uncategorized descriptions with how often they occur; manually uncategorized
-- rows are counted in total_count but are out of reach of rules
select
  t.tx_desc::text as tx_desc,
  count(*) filter (where not t.category_manually_set)::bigint as eligible_count,
  count(*)::bigint as total_count,
  max(t.tx_date)::timestamptz as last_seen
from transactions t
join accounts a on a.id = t.account_id
where a.owner_id = @user_id::uuid
  and a.account_type != 6
  and t.category_id is null
  and t.tx_desc is not null
  and not exists (
    select 1 from transfers tr
    where tr.status = 1 and (tr.out_tx_id = t.id or tr.in_tx_id = t.id)
  )
group by t.tx_desc;

-- name: ListCategorizedDescriptions :many
select
  t.tx_desc::text as tx_desc,
  t.category_id::bigint as category_id,
  count(*)::bigint as tx_count
from transactions t
join accounts a on a.id = t.account_id
where a.owner_id = @user_id::uuid
  and a.account_type != 6
  and t.category_id is not null
  and t.tx_desc is not null
group by t.tx_desc, t.category_id;
