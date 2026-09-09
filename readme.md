# nagomi-core

nagomi-core is a high-performance gRPC (or rather [connect-go](https://github.com/connectrpc/connect-go)) API built in Go that handles core financial operations including user management, account management, transaction processing, and categorization, among other things.

## ⚙️ config

### environment variables

| variable                  | description                                | default              | required?  |
|---------------------------|--------------------------------------------|----------------------|------------|
| `API_KEY`                 | Authentication key for gRPC API access     |                      | [x]        |
| `DATABASE_URL`            | PostgreSQL connection string               |                      | [x]        |
| `NAGOMI_GATEWAY_URL`        | URL for nagomi-gateway (auth + proxy)        |                      | [x]        |
| `NAGOMI_RECEIPTS_URL`       | gRPC endpoint for receipt parsing service  |                      | [x]        |
| `EXCHANGE_API_URL`        | Exchange rate API endpoint                 |                      | [x]        |
| `S3_ENDPOINT`             | S3-compatible endpoint URL (Garage/B2/...) |                      | [x]        |
| `S3_BUCKET`               | Bucket used for receipt images             |                      | [x]        |
| `S3_ACCESS_KEY`           | S3 access key ID                           |                      | [x]        |
| `S3_SECRET_KEY`           | S3 secret access key                       |                      | [x]        |
| `S3_REGION`               | Region string                              |                      | [x]        |
| `LISTEN_ADDRESS`          | Server listen address (port or host:port)  | `127.0.0.1:55555`    | [ ]        |
| `LOG_LEVEL`               | Log level: debug, info, warn, error        | `info`               | [ ]        |
| `LOG_FORMAT`              | Log format: json, text                     | `text`               | [ ]        |

## 🌱 ecosystem

- [nagomi-core](https://github.com/xhos/nagomi-core) - main backend service (this repo)
- [nagomi-web](https://github.com/xhos/nagomi-web) - frontend web application
- [nagomi-mobile](https://github.com/xhos/nagomi-mobile) - mobile appplication
- [nagomi-protos](https://github.com/xhos/nagomi-protos) - shared protobuf definitions
- [nagomi-receipts](https://github.com/xhos/nagomi-receipts) - receipt parsing microservice
- [nagomi-email-parser](https://github.com/xhos/nagomi-email-parser) - email parsing service


nagomi-web is the expected frontend to use, but it is possible to build your own client. The only thing tightly coupled is the Better Auth JWT authentication mechanism, but you can use inter-service API keys to authenticate instead if you prefer.
