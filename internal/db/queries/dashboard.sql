-- name: GetDashboardTrends :many
select
  to_char(t.tx_date::date, 'YYYY-MM-DD') as date,
  SUM(case when t.tx_direction = 1 then r.report_cents else 0 end)::bigint as income_cents,
  SUM(case when t.tx_direction = 2 then r.report_cents else 0 end)::bigint as expense_cents
from transactions t
join transaction_report r on r.id = t.id
join accounts a on t.account_id = a.id
where a.owner_id = @user_id::uuid
  and a.account_type != 6
  and (sqlc.narg('start')::timestamptz is null or t.tx_date >= sqlc.narg('start')::timestamptz)
  and (sqlc.narg('end')::timestamptz is null or t.tx_date <= sqlc.narg('end')::timestamptz)
group by date
order by date;

-- name: GetDashboardSummary :one
select
  COUNT(distinct a.id)::bigint as total_accounts,
  COUNT(t.id)::bigint as total_transactions,
  COALESCE(SUM(case when t.tx_direction = 1 then r.report_cents else 0 end), 0)::bigint as total_income_cents,
  COALESCE(SUM(case when t.tx_direction = 2 then r.report_cents else 0 end), 0)::bigint as total_expense_cents,
  COUNT(distinct case when t.tx_date >= CURRENT_DATE - interval '30 days' then t.id end)::bigint as transactions_last_30_days,
  COUNT(distinct case when t.category_id is null and not r.is_transfer then t.id end)::bigint as uncategorized_transactions
from accounts a
left join transactions t on a.id = t.account_id
left join transaction_report r on r.id = t.id
where a.owner_id = @user_id::uuid
  and a.account_type != 6
  and (sqlc.narg('start')::timestamptz is null or t.tx_date >= sqlc.narg('start')::timestamptz)
  and (sqlc.narg('end')::timestamptz is null or t.tx_date <= sqlc.narg('end')::timestamptz);

-- name: GetTopCategories :many
select
  c.slug,
  c.color,
  COUNT(t.id)::bigint as transaction_count,
  SUM(r.report_cents)::bigint as total_amount_cents
from transactions t
join transaction_report r on r.id = t.id
join categories c on t.category_id = c.id
join accounts a on t.account_id = a.id
where a.owner_id = @user_id::uuid
  and a.account_type != 6
  and t.tx_direction = 2
  and r.report_cents > 0
  and (sqlc.narg('start')::timestamptz is null or t.tx_date >= sqlc.narg('start')::timestamptz)
  and (sqlc.narg('end')::timestamptz is null or t.tx_date <= sqlc.narg('end')::timestamptz)
group by c.id, c.slug, c.color
order by total_amount_cents desc
limit COALESCE(sqlc.narg('limit')::int, 10);

-- name: GetTopMerchants :many
select
  t.merchant,
  COUNT(t.id)::bigint as transaction_count,
  SUM(t.tx_amount_cents)::bigint as total_amount_cents,
  AVG(t.tx_amount_cents)::bigint as avg_amount_cents
from transactions t
join transaction_report r on r.id = t.id
join accounts a on t.account_id = a.id
where a.owner_id = @user_id::uuid
  and a.account_type != 6
  and t.merchant is not null
  and not r.is_transfer
  and t.tx_direction = 2
  and (sqlc.narg('start')::timestamptz is null or t.tx_date >= sqlc.narg('start')::timestamptz)
  and (sqlc.narg('end')::timestamptz is null or t.tx_date <= sqlc.narg('end')::timestamptz)
group by t.merchant
order by total_amount_cents desc
limit COALESCE(sqlc.narg('limit')::int, 10);

-- name: GetMonthlyComparison :many
select
  to_char(t.tx_date, 'YYYY-MM') as month,
  SUM(case when t.tx_direction = 1 then r.report_cents else 0 end)::bigint as income_cents,
  SUM(case when t.tx_direction = 2 then r.report_cents else 0 end)::bigint as expense_cents,
  SUM(case when t.tx_direction = 1 then r.report_cents else -r.report_cents end)::bigint as net_cents
from transactions t
join transaction_report r on r.id = t.id
join accounts a on t.account_id = a.id
where a.owner_id = @user_id::uuid
  and a.account_type != 6
  and t.tx_date >= COALESCE(sqlc.narg('start')::timestamptz, CURRENT_DATE - interval '12 months')
  and t.tx_date <= COALESCE(sqlc.narg('end')::timestamptz, CURRENT_DATE)
group by month
order by month;

-- name: GetAccountBalances :many
select
  a.id,
  a.name,
  a.account_type,
  a.anchor_currency as currency,
  COALESCE(
    (select t.balance_after_cents
     from transactions t
     where t.account_id = a.id
     order by t.tx_date desc, t.id desc
     limit 1),
    a.anchor_balance_cents
  ) as balance_cents
from accounts a
where a.owner_id = @user_id::uuid
  and a.account_type != 6
order by
  case a.account_type
    when 1 then 1
    when 2 then 2
    when 3 then 3
    when 4 then 4
    when 5 then 5
    else 6
  end,
  balance_cents desc;

-- name: GetNetWorthHistory :many
with date_series as (
  select
    generate_series(
      @start_date::timestamptz,
      @end_date::timestamptz,
      case @granularity::int
        when 1 then interval '1 day'
        when 2 then interval '1 week'
        when 3 then interval '1 month'
        else interval '1 day'
      end
    )::date as period_date
),
account_balances_at_date as (
  select
    ds.period_date,
    a.id as account_id,
    a.anchor_currency,
    CASE
      -- If there's a transaction on or before the period date, use its balance
      WHEN EXISTS (
        select 1 from transactions t
        where t.account_id = a.id and t.tx_date <= ds.period_date
      ) THEN (
        select t.balance_after_cents
        from transactions t
        where t.account_id = a.id and t.tx_date <= ds.period_date
        order by t.tx_date desc, t.id desc
        limit 1
      )
      -- If anchor date is on or before period date, use anchor balance
      WHEN a.anchor_date <= ds.period_date THEN a.anchor_balance_cents
      -- Otherwise, account didn't exist yet, so balance is 0
      ELSE 0
    END as balance_cents
  from date_series ds
  cross join accounts a
  where a.owner_id = @user_id::uuid
    and a.account_type != 6
)
select
  to_char(ab.period_date, 'YYYY-MM-DD') as date,
  SUM(ab.balance_cents)::bigint as net_worth_cents
from account_balances_at_date ab
group by ab.period_date
order by ab.period_date;

-- name: GetEarliestTransactionDate :one
select MIN(t.tx_date)::date as earliest_date
from transactions t
join accounts a on t.account_id = a.id
where a.owner_id = @user_id::uuid
  and a.account_type != 6;
