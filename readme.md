# nagomi-core

Backend for [nagomi](https://github.com/xhos/nagomi). A [connect-go](https://github.com/connectrpc/connect-go) API over postgres that owns users, accounts, transactions, categories, rules, receipts and statements.

Clients reach it through [nagomi-gateway](https://github.com/xhos/nagomi-gateway). Other services use `API_KEY`.

## config

See [.env.example](.env.example).

## dev

```bash
run          # hot-reloading server
migrate      # apply migrations to $DATABASE_URL
regen        # regenerate sqlc and proto code
test-db      # db tests against $TEST_DATABASE_URL, or $DATABASE_URL
cover
bump-protos
```
