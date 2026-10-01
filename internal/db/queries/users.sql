-- name: ListUsers :many
select * from users
order by created_at desc;

-- name: GetUser :one
select * from users
where id = @id::uuid;

-- name: GetUserByEmail :one
select * from users
where lower(email) = lower(@email::text);

-- name: CreateUser :one
insert into users (id, email, display_name)
values (@id::uuid, @email::text, sqlc.narg('display_name')::text)
returning *;

-- name: UpdateUser :exec
update users
set
  email = sqlc.narg('email')::text,
  display_name = sqlc.narg('display_name')::text,
  primary_currency = coalesce(sqlc.narg('primary_currency')::varchar(3), primary_currency),
  timezone = coalesce(sqlc.narg('timezone')::varchar(50), timezone)
where id = @id::uuid;

-- name: DeleteUser :execrows
delete from
  users
where
  id = @id::uuid;

-- name: UpsertUser :one
insert into users (id, email, display_name)
values (@id::uuid, @email::text, sqlc.narg('display_name')::text)
on conflict (id) do update set
  email = excluded.email,
  display_name = coalesce(excluded.display_name, users.display_name),
  updated_at = now()
returning *;

-- name: GetUserPrimaryCurrency :one
select
  primary_currency
from
  users
where
  id = @id::uuid;

-- name: GetUserTimezone :one
select
  timezone
from
  users
where
  id = @id::uuid;
