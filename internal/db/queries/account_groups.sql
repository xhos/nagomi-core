-- name: ListAccountGroups :many
select
  *
from
  account_groups
where
  owner_id = @user_id::uuid
order by
  name;

-- name: CreateAccountGroup :one
insert into
  account_groups (owner_id, name, color)
values
  (
    @user_id::uuid,
    @name::text,
    coalesce(sqlc.narg('color')::text, '#3b82f6')
  )
returning
  *;

-- name: UpdateAccountGroup :execrows
update
  account_groups
set
  name = coalesce(sqlc.narg('name')::text, name),
  color = coalesce(sqlc.narg('color')::text, color)
where
  id = @id::bigint
  and owner_id = @user_id::uuid;

-- name: DeleteAccountGroup :execrows
delete from
  account_groups
where
  id = @id::bigint
  and owner_id = @user_id::uuid;

-- name: SetAccountsGroup :execrows
-- a null group takes the accounts out of any; a group must be the user's own
update
  accounts
set
  group_id = sqlc.narg('group_id')::bigint
where
  owner_id = @user_id::uuid
  and id = ANY(@account_ids::bigint [])
  and (
    sqlc.narg('group_id')::bigint is null
    or exists (
      select 1 from account_groups g
      where g.id = sqlc.narg('group_id')::bigint and g.owner_id = @user_id::uuid
    )
  );

-- name: DeleteEmptyAccountGroups :exec
delete from
  account_groups g
where
  g.owner_id = @user_id::uuid
  and not exists (select 1 from accounts a where a.group_id = g.id);
