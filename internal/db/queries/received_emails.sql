-- name: InsertReceivedEmail :exec
insert into received_emails (user_id, sender, subject, outcome, transaction_id, error, body)
values (
  @user_id::uuid,
  @sender::text,
  @subject::text,
  @outcome,
  sqlc.narg('transaction_id')::bigint,
  sqlc.narg('error')::text,
  sqlc.narg('body')::text
);

-- name: TrimReceivedEmails :exec
delete from received_emails
where user_id = @user_id::uuid
  and id not in (
    select id
    from received_emails
    where user_id = @user_id::uuid
    order by received_at desc, id desc
    limit @keep::int
  );

-- name: ListReceivedEmails :many
select *
from received_emails
where user_id = @user_id::uuid
order by received_at desc, id desc;
