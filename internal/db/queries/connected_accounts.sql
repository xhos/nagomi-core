-- name: ListDueSyncJobs :many
select *
from connected_accounts
where status in ('active', 'broken') and next_run_at is not null and next_run_at <= now()
order by next_run_at;

-- name: GetConnectedAccount :one
select *
from connected_accounts
where id = @id::bigint;

-- name: CompleteSyncJob :exec
update connected_accounts
set sync_cursor = coalesce(sqlc.narg('sync_cursor')::timestamptz, sync_cursor),
    status      = coalesce(sqlc.narg('status')::text, status),
    next_run_at = case
      when sync_interval_minutes is not null
        then now() + (sync_interval_minutes * interval '1 minute')
      else null
    end
where id = @id::bigint;

-- name: CreateConnectedAccount :one
insert into connected_accounts (user_id, provider, credentials, sync_interval_minutes)
values (@user_id::uuid, @provider::text, @credentials::bytea, sqlc.narg('sync_interval_minutes')::int)
returning *;

-- name: DeleteConnectedAccount :execrows
delete from connected_accounts
where id = @id::bigint and user_id = @user_id::uuid;

-- name: ListConnectionsForUser :many
select id, provider, status, sync_cursor, sync_interval_minutes, next_run_at, created_at
from connected_accounts
where user_id = @user_id::uuid
order by created_at desc;

-- name: TriggerSync :execrows
update connected_accounts
set next_run_at = now(), status = 'active'
where id = @id::bigint and user_id = @user_id::uuid;

-- name: SetSyncInterval :execrows
update connected_accounts
set sync_interval_minutes = sqlc.narg('sync_interval_minutes')::int,
    next_run_at = case
      when sqlc.narg('sync_interval_minutes')::int is null then null
      when sqlc.narg('sync_interval_minutes')::int is not null
        then now() + (sqlc.narg('sync_interval_minutes')::int * interval '1 minute')
      else next_run_at
    end
where id = @id::bigint and user_id = @user_id::uuid;
