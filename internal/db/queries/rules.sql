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
  rule_name = sqlc.narg('rule_name'),
  category_id = sqlc.narg('category_id')::bigint,
  conditions = sqlc.narg('conditions'),
  is_active = sqlc.narg('is_active'),
  priority_order = sqlc.narg('priority_order'),
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
  and (sqlc.narg('include_manually_set')::boolean = true or (t.category_manually_set = false and t.merchant_manually_set = false));

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