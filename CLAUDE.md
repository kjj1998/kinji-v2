# CLAUDE.md

Personal expenses tracker that records each payment as it happens, starting with Apple Pay.
It is a full rework of https://github.com/kjj1998/kinji (React SPA + Go BFF + SQLite), and
doubles as an AI engineering portfolio project.

Read `docs/design.md` before making architectural changes. It records what was decided,
why, what was rejected, and what is still open. Update it when a decision changes.

## Stack

- **Web app:** TanStack Start (TypeScript, React, Mantine), runs on a DigitalOcean droplet behind Caddy.
  Owns auth, API token issuance, dashboard, and the review inbox. Not in the capture path.
- **Lambdas:** Go on AWS Lambda, `provided.al2023` runtime, arm64, binary named `bootstrap`.
  - `ingest`: Function URL that the iPhone Shortcut calls. Stores raw events.
  - `enrich`: SQS consumer. Parses and categorizes events with an LLM, writes transactions.
  - `sweeper`: EventBridge Scheduler job. Re-queues events stuck in `pending`.
- **Queue:** SQS standard queue with a dead-letter queue.
- **Database:** Postgres on Neon (free plan, `ap-southeast-1`). Use the pooled connection string.
- **Infra:** Terraform. AWS region `ap-southeast-1`. S3 state backend with `use_lockfile`.
- **CI/CD:** GitHub Actions, authenticating to AWS through GitHub OIDC (no long-lived AWS keys).

## Repo layout

```
/web            TanStack Start app
/lambdas        Go module
  cmd/ingest    Function URL handler
  cmd/enrich    SQS consumer
  cmd/sweeper   re-queues stuck events
  cmd/mint-token  CLI that creates a user and prints an ingest token (bootstrap, until the web app exists)
  internal/     sqlc-generated DB code, event types, LLM client, shared logic
/db/migrations  SQL migrations: the single source of truth for the schema
/infra          Terraform
/docs           design notes
```

## Commands

Terraform version is pinned in `mise.toml`. Each folder under `/infra` is its own root module
with its own state.

**Each `/infra` folder must use its own backend `key`** (`<folder>/terraform.tfstate`). When
creating a stack by copying another's `versions.tf`, change the key. A shared key makes one stack
plan to destroy the other's resources, because they are in state but not in its config.
Read the plan summary line before applying; don't use `-auto-approve` on an unreviewed plan.

**`/infra/bootstrap`** (state bucket + budget alert; applied rarely, state in its own S3 bucket):

```sh
aws sso login --profile admin && export AWS_PROFILE=admin
terraform -chdir=infra/bootstrap init
terraform -chdir=infra/bootstrap plan
terraform -chdir=infra/bootstrap apply
```

Needs `infra/bootstrap/terraform.tfvars` (gitignored) with `alert_email`. Backend blocks only
take literals, so the bucket name is hardcoded in `versions.tf`.

**`/infra/main`** (app infrastructure: SSM parameters now, Lambdas and SQS later; state key
`main/terraform.tfstate`):

```sh
terraform -chdir=infra/main init
terraform -chdir=infra/main plan
terraform -chdir=infra/main apply
```

Secret SSM parameters use write-only `value_wo = "placeholder"` with `value_wo_version = 1`;
the real value is set in the console and never reaches state. Don't bump `value_wo_version`
(it resets the value to the placeholder), and don't use `value` + `ignore_changes` (refresh
reads the decrypted value into state).

Still to be filled in: build, test, migrate, deploy for `/web`, `/lambdas`, `/db`.

## Conventions

- **Money** is stored as integer cents with a currency code (default `SGD`). Never use floats for amounts.
- **Timestamps** are stored in UTC. Display in Singapore time (+08:00).
- **Schema** changes only go through SQL files in `/db/migrations`. Go types come from sqlc and
  TypeScript types come from the same schema (Drizzle or Kysely). Do not let a TypeScript ORM own migrations.
- **Raw first:** the ingest Lambda stores payloads exactly as received. All parsing and cleanup
  happens in `enrich`. Raw payloads are kept so events can be replayed for evals.
- **Idempotency everywhere:** SQS delivers at least once, the phone may retry, and one purchase can
  arrive from several sources. Every write must be safe to repeat.
- **LLM calls sit behind an interface**, so logic can be tested without calling the API and the
  provider can be swapped (e.g. a self-hosted OpenAI-compatible model later).
- **Each component has its own Postgres role and IAM role** with the minimum permissions it needs.
- Categories start from Kinji's list: Entertainment, Food, Groceries, Health, Income, Shopping,
  Subscriptions, Transport, Utilities, Credit, Transfer, Taxes, Accommodation, Insurance.

## Rules

- Never log the `Authorization` header, API tokens, connection strings, or API keys.
- Never commit secrets. Secrets live in SSM Parameter Store as `SecureString` (not Secrets Manager).
- Never store or forward one-time passwords or security codes from bank notifications.
  Ingest drops anything that looks like an OTP before storing it.
- The ingest Lambda never calls the LLM and must return quickly (202).
- Ask before adding anything that costs money at rest: NAT gateways, VPCs for the Lambdas, RDS or
  Aurora, Secrets Manager, provisioned concurrency, or always-on compute.
- Keep `ingest` reserved concurrency low (around 5) and CloudWatch log retention set (around 14 days).
