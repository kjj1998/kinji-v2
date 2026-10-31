# Design notes

Decisions, reasoning, and open questions for the expenses tracker rework. Each decision is
marked **Decided** (chosen explicitly) or **Proposed** (recommended and not yet pushed back on,
but not explicitly confirmed). Revisit Proposed items before building on them.

## 1. Goal and scope

Record each payment as it happens instead of waiting for the monthly bank or card statement.
Kinji (the previous version) only imported transactions by having an LLM extract them from
uploaded PDF statements. Statements stay useful, but as the monthly source of truth that
real-time captures are checked against.

The project is also an AI engineering portfolio vehicle, so features should be real and used,
and their quality should be measurable (see section 7).

**Current focus (Decided): Apple Pay only.** Other payment flows come later (section 8).

## 2. Architecture

```
iPhone (Shortcuts)
  Wallet trigger (in-store taps)
  Notification trigger (iOS 27: Wallet app, bank apps)
        |
        |  HTTPS POST, Authorization: Bearer <ingest token>
        v
Ingest Lambda (Go, Function URL) ---- insert raw event ----> Postgres (Neon)
        |                                                     ^      ^
        |  send {event_id}                                    |      |
        v                                                     |      |
SQS queue ---- after ~5 failed receives ----> dead-letter queue      |
        |                                                     |      |
        v                                                     |      |
Enrich Lambda (Go) ---- LLM API; writes transaction ----------+      |
                                                                     |
Sweeper Lambda (EventBridge Scheduler) re-queues stale pending events|
                                                                     |
TanStack Start app (DigitalOcean droplet, Caddy) --------------------+
  issues API tokens, dashboard, review inbox; not in the capture path
```

The TypeScript app and the Go Lambdas meet only at the Postgres schema and the
`api_tokens` table (the app creates tokens; ingest verifies them).

## 3. Components

### iPhone Shortcuts (capture)

- **Wallet automation** (Decided): Shortcuts > Automation > + > Wallet. Called "Transaction"
  before iOS 26. Needs iOS 17+. Pick the cards to track, enable Run Immediately.
  The Shortcut Input is a transaction with fields Transaction, Card, Merchant, Amount, Name.
  Action: Get Contents of URL, POST, JSON body, `Authorization: Bearer <token>` header.
- **Notification automation** (Proposed): iOS 27 "When I receive a notification from [app]",
  with up to three filters on message, title, subtitle. Point it at the Wallet app and the card
  issuer's app. Sends app name, title, subtitle, body, received time.
- Optional local journal: an "Append to Text File" action writing the same JSON to iCloud Drive
  before the POST, so failed requests can be replayed.

Known limitations of the Wallet trigger (reported by other developers):
- Fires only for in-store contactless taps. In-app and website Apple Pay do not trigger it.
  This is why the notification trigger is planned.
- Merchant sometimes arrives blank and amount as 0.0.
- Often times out waiting for Wallet's transaction record, which can arrive hours late.
- Needs mobile data enabled for the Wallet app.
- Shortcuts do not retry. A failed POST loses the event until statement reconciliation.

### Ingest Lambda (Go, Function URL, auth type NONE)

1. Hash the bearer token (SHA-256), look it up in `api_tokens` (scope `ingest`, not revoked).
   Unknown or missing token: 401, no DB write.
2. Drop anything that looks like an OTP or security code (notification sources).
3. Insert the raw payload into `capture_events` with a dedupe key, `ON CONFLICT DO NOTHING`.
   A duplicate still returns 202.
4. Send `{event_id}` to SQS. If the send fails, still return 202: the event is durable in
   Postgres and the sweeper will re-queue it.
5. Return 202. Never call the LLM here.

Settings: reserved concurrency ~5 (it is a public URL), CloudWatch log retention ~14 days,
pgx pool created outside the handler and reused across warm invocations, Neon pooled
connection string read from SSM Parameter Store at cold start.

Dedupe key (Proposed): hash of user, source, and the source's identifying fields with the
timestamp truncated to the minute. For Wallet taps: card, merchant, amount, `tapped_at`.
For notifications: app, title, body, received time.

### SQS queue and dead-letter queue

- Standard queue. Messages carry only `{event_id}`; the event itself lives in Postgres.
- Redrive to the DLQ after about 5 receives.
- Visibility timeout at least 6x the enrich function timeout.

### Enrich Lambda (Go, SQS trigger)

- Batch processing with `ReportBatchItemFailures`, so one bad message does not retry the batch.
- Timeout around 60 s to allow for the LLM call.
- Per event: load raw payload, parse amount and currency, check `merchant_aliases`, call the
  LLM only for merchants not seen before or for free-text notifications, then write or update
  a transaction.
- Cross-source matching: one purchase can produce a Wallet tap event, a Wallet notification,
  and a bank notification. Attach a new event to an existing transaction when card, amount,
  and time window match, instead of creating a duplicate.
- Writes must be idempotent (at-least-once delivery).
- Blank merchant or zero amount from the Wallet trigger: store as `needs_review`, do not drop.
- For notification text the LLM must also classify the message (completed purchase, refund,
  declined, promotion, other) and return a confidence score. Low confidence goes to review.

### Sweeper Lambda

EventBridge Scheduler, every few minutes. Re-queues `capture_events` still `pending` after a
threshold. A lightweight outbox pattern: covers the gap when ingest wrote the event but
the SQS send failed.

### Postgres (Neon free plan) (Proposed)

- Region `ap-southeast-1`, same as the Lambdas.
- Free plan at time of decision (Oct 2026): permanent, no card, 1 GB storage per project,
  100 CU-hours per project per month, compute scales to zero after 5 minutes idle and resumes in
  a few hundred ms.
- pgvector available for later merchant similarity and semantic search.
- One Postgres role per component: `ingest` (insert events, read token hashes), `enrich`,
  `sweeper`, `app`, and a migration role.

### Web app (TanStack Start on the existing droplet) (Proposed)

- Keeps the current DigitalOcean droplet and Caddy setup.
- Auth via Better Auth (Proposed) instead of hand-rolled sessions. Requirement: username and
  password login now; Google, GitHub and Microsoft social login later. Better Auth covers both
  (email/password plus the `username` plugin; social providers are config). The `users` table in
  `00001_init.sql` already matches its user model, mapped to snake_case, so `api_tokens`
  references the right table from day one. Its `session`, `account` and `verification` tables
  arrive in a later migration with the web app; passwords and social logins both live in
  `account`, so adding a provider needs no schema change.
- Settings page to create and revoke ingest tokens (shown once, stored hashed).
- Review inbox for provisional, flagged, and `needs_review` transactions. Category corrections
  update `merchant_aliases` and are logged as labelled data.
- Installable as a PWA (home-screen web apps on iOS support push notifications, for later
  "what was this for?" prompts).

## 4. Data model (draft)

Milestone 1 needs only `users`, `api_tokens`, and `capture_events`.

```
users             id (uuid v7), name, email, email_verified, image, username,
                  display_username, created_at, updated_at   (Better Auth's user model)

api_tokens        id, user_id, token_hash (unique), scope ('ingest'), name,
                  created_at, last_used_at, revoked_at

capture_events    id (uuid v7), user_id, source ('wallet_tap' | 'wallet_notification' |
                  'bank_notification'), payload (jsonb, exactly as received),
                  dedupe_key (unique), status ('pending' | 'processing' | 'done' |
                  'failed' | 'discarded'), attempts, last_error, received_at, processed_at

transactions      id, user_id, occurred_at, merchant_raw, merchant, amount_cents, currency,
                  direction ('INFLOW' | 'OUTFLOW'), type ('expense' | 'income' | 'transfer'),
                  category, card, status ('provisional' | 'confirmed' | 'flagged' |
                  'needs_review'), confidence, notes, created_at, updated_at

transaction_sources  transaction_id, capture_event_id   (many events -> one transaction;
                  statement lines join here too once reconciliation exists)

merchant_aliases  user_id, raw_name, merchant, category, origin ('user' | 'llm'), created_at
```

Transaction `type` matters once bank and card statements are both ingested: paying the credit
card bill from the bank account is a `transfer`, not an expense. Counting it would double every
card purchase. The same applies to e-wallet top-ups and moves between own accounts.

Transaction lifecycle: `provisional` when captured in real time, `confirmed` when matched to a
statement line, `flagged` when it never appears on the statement, `needs_review` when data is
incomplete or confidence is low.

## 5. Key decisions and why

| Decision | Status | Why |
|---|---|---|
| Full rework instead of evolving Kinji | Decided | Owner wants frontend and backend in one language; Kinji's React SPA + Go BFF split also duplicated types and endpoints across both |
| TypeScript full-stack app (TanStack Start) | Decided: TS full-stack. Proposed: TanStack Start over Next.js | Start is the smallest jump from Kinji's Vite + React + TanStack Query stack; Server Components are optional; Mantine runs client-side and fits awkwardly in Next's App Router. Next.js remains the alternative with more jobs and ecosystem. TanStack Start 1.0 went stable on 13 Oct 2026 |
| Worker in Go on Lambda | Decided | Go fits the infra/MLOps direction. TS and Go meet only at a queue message and the schema, a much smaller contract than Kinji's request/response boundary |
| Shortcut posts to an ingest Lambda, not the web app | Decided | Capture keeps working if the droplet is down or mid-deploy |
| Store raw payloads before any parsing | Proposed | Nothing is lost when the LLM is slow or down; events can be replayed when prompts or models change; real payloads become eval cases |
| Neon free plan for Postgres | Proposed | $0, reachable over TLS from Lambda without a VPC, pgvector, free branches for test databases |
| SQS + DLQ + sweeper | Proposed | Retries and failure visibility without running servers; sweeper covers a failed SQS send after the DB write |
| Wallet trigger first, notification trigger next | Proposed | Wallet trigger gives structured fields; notifications cover in-app and web Apple Pay and late Wallet records |
| Postgres 18 on Neon | Decided | Built-in `uuidv7()` for time-ordered ids, no extension needed |
| Better Auth for web app auth | Proposed | Username/password now, Google/GitHub/Microsoft later without schema changes; schema stays in goose migrations rather than its CLI |
| goose for migrations | Decided | Go-native, so migrations can be embedded in a Go binary for CI; sqlc reads goose's annotated SQL files and ignores the Down sections. dbmate was the alternative |
| Two milestones: raw capture first, enrichment second | Proposed | Proves the riskiest parts (payload, auth, Lambda to Neon) and collects real payloads before writing parsers |

## 6. Milestones and setup dependencies

### Milestone 1: tap to raw row in `capture_events`

| # | What | Depends on | Notes |
|---|---|---|---|
| 1 | AWS account, budget alert, admin access for Terraform | - | **Done 2026-10-25.** Budget alert first; a public URL is the one way this can cost money. `infra/bootstrap`: $5/month COST budget, emails at 80% actual and 100% forecasted. Terraform runs as SSO profile `admin` |
| 2 | Terraform state backend: S3 bucket with `use_lockfile` | 1 | **Done 2026-10-25.** No DynamoDB lock table needed. Bucket `kinji-v2-tfstate-271407076537` (versioned, SSE, public access blocked, TLS-only policy, noncurrent versions expire after 90 days); bootstrap state at `bootstrap/terraform.tfstate` |
| 3 | Neon project in `ap-southeast-1` | - | **Done 2026-10-27.** Can be done in parallel with 1-2. Created by hand in the console, not Terraform |
| 4 | Migrations: `users`, `api_tokens`, `capture_events` | 3 | **Done 2026-10-27** (`00001_init.sql`). goose, sequential SQL files in `db/migrations`, run over Neon's direct (unpooled) connection string |
| 5 | Per-component Postgres roles | 4 | **Done 2026-10-28** (`00002_roles.sql`: `ingest`, `mint_token`). Created NOLOGIN by SQL, login and password enabled by hand |
| 6 | `cmd/mint-token` CLI | 4, 5 | **Done 2026-10-31.** `create` / `revoke` subcommands; connects as `mint_token` with the URL from SSM. Creates the user and prints a token once; replaces the web app's settings page for now. Token format (Decided): `kji_` + base64url (no padding) of 32 random bytes; `token_hash` is SHA-256 of the full string including the prefix. The prefix makes leaked tokens easy to grep and to flag in secret scanners |
| 7 | SSM SecureString with the ingest role's pooled connection string | 1, 5 | **Done 2026-10-28.** Also `/kinji-v2/mint-token/database-url` for the CLI, read at run time so the passwords live only in SSM. First resource in the main stack (`infra/main`, state key `main/terraform.tfstate`). Terraform creates `/kinji-v2/ingest/database-url` with write-only `value_wo = "placeholder"` and `value_wo_version = 1`; the real value is then set in the console. Write-only values never reach state, unlike `value` + `ignore_changes`, which still reads the decrypted value back into state on refresh. Bumping `value_wo_version` (or replacing the resource) resets it to the placeholder |
| 8 | Ingest Lambda code | 4 | |
| 9 | Terraform: IAM role, function, Function URL, reserved concurrency, log retention | 2, 7, 8 | Role can only write logs and read its own parameter |
| 10 | Wallet automation in Shortcuts | 6, 9 | Test the deployed function with `curl` first |

After step 10: pay normally for about a week and inspect real rows before writing parsers.

### Milestone 2: raw event to categorized transaction

| # | What | Depends on | Notes |
|---|---|---|---|
| 11 | SQS queue + DLQ | 2 | |
| 12 | Ingest sends `{event_id}`; grant it `sqs:SendMessage` on that queue only | 9, 11 | |
| 13 | `transactions`, `transaction_sources`, `merchant_aliases`; enrich role | 4 | |
| 14 | LLM API key in SSM | 1 | |
| 15 | Enrich Lambda with SQS trigger | 11, 13, 14 | Batch failures, visibility timeout 6x function timeout |
| 16 | Sweeper Lambda + EventBridge Scheduler | 11, 13 | |
| 17 | GitHub Actions: test, build, Terraform, migrations, via GitHub OIDC | after 9 | |

The notification trigger can be added any time after milestone 1: a second Shortcuts automation
on iOS 27 plus a `source` field in ingest. The enrich step then needs to parse notification text.

## 7. Evals and measurement

- **Capture accuracy:** each monthly statement grades real-time capture. Report matched,
  missed (on statement, never captured), and flagged (captured, not on statement), per source.
- **Source comparison:** after a few weeks of running both triggers, compare which source
  caught which payments and how late. Decide from data whether the Wallet trigger is still needed.
- **Extraction accuracy:** amount, merchant, and card extracted from notification text, checked
  against confirmed transactions.
- **Categorization:** user corrections in the review inbox are labelled data.
- Later: tracing and cost per event; a local model for extraction compared against the API on
  the same eval set; possibly fine-tuning a small categorizer on accumulated corrections.

## 8. Later slices

### Statement reconciliation (next after Apple Pay)

Kinji's PDF extraction becomes reconciliation, and runs asynchronously: upload PDF to S3, the
S3 event triggers a Go Lambda that extracts and reconciles, the UI polls for status. (Kinji
parsed synchronously inside the upload request, which can exceed serverless request limits.)

Match statement lines to provisional transactions on: same card, exact amount (or close for
foreign currency), date within a couple of days, similar merchant name. Matched become
`confirmed`; unmatched statement lines are inserted as missed captures; unmatched provisional
transactions become `flagged`.

### Other payment flows

| Flow | Likely capture channel | Notes |
|---|---|---|
| Apple Pay tap (credit cards) | Wallet trigger, Wallet/bank notification | Current focus |
| PayNow QR | Bank outgoing-payment alert (push via notification trigger, or email) | Payee is often the business's registered name, not the stall name: needs merchant aliases |
| Shopee / Lazada | Order confirmation (email or app notification) + card/wallet alert | One charge, many items; vouchers mean item prices don't sum to the charge; refunds come later |
| SimplyGo EZ-Link top-up | Card alert or top-up receipt | Count the top-up as transport spend, never also the fares |
| PayNow to friends (bill splits) | Bank outgoing-payment alert | Needs a "what was this for?" prompt; incoming reimbursements should link to the original charge |
| GIRO deductions (insurance) | Expected entry from schedule; bank alert if sent | Confirm on match; flag if amount changes or it is missing |

Singapore banks must send outgoing-payment alerts above a user-set threshold (MAS E-Payments
User Protection Guidelines); setting the threshold to the minimum makes alerts a reliable channel.

### Telegram bot (distant)

Read access to expenses from Telegram instead of building a mobile app. No password login:
Telegram identifies the sender of every update (numeric user id), and the webhook's secret
token proves the update came from Telegram. Auth is a one-time link from a logged-in web
session: settings page issues a short-lived, single-use code as `t.me/<bot>?start=<code>`; the
bot receives `/start <code>` and stores telegram user id ↔ `users.id`. Unlinked senders get nothing.

- Link storage: a Better Auth `account` row (`provider_id = 'telegram'`) or a dedicated
  `telegram_links` table. Either is a new migration; `users` does not change.
- `users.email` is NOT NULL, so Telegram-only sign-up without an email is not supported. Linking
  an existing account is the intended flow.
- Its own Postgres role, read-only on transactions. Private chats only, never groups.

## 9. Open questions

- What does the Wallet trigger actually send for `amount` (e.g. "S$12.30" vs "12.3"),
  `merchant`, and `name`? How do overseas charges look? Answer from real `capture_events` rows.
- Do the Wallet app and each card issuer's app send a notification for in-app and web
  Apple Pay? What does the text look like?
- Can the iOS 27 notification trigger target the Wallet app on this phone?
- Time window for cross-source matching, given Wallet records can arrive hours late.
- TS DB library (Drizzle vs Kysely).
- Confirm Better Auth when the web app starts (Proposed; see Web app in section 3).
- Whether to add a custom domain for ingest later (Function URLs need CloudFront for that).
  The raw Function URL is stable for the life of the function, so it is fine in the Shortcut for now.

## 10. Rejected alternatives

| Alternative | Why not |
|---|---|
| Evolve Kinji in place | Owner wants one language for UI and server and a clean rework |
| Shortcut posts to the web app's ingest route | Capture would depend on the droplet being up |
| Shortcut sends directly to SQS | Shortcuts can't realistically sign AWS requests, and storing an AWS secret on the phone is a bad idea; the only copy would live in SQS (max 14 days) |
| API Gateway to SQS service integration | API keys aren't meant for auth; raw event wouldn't reach Postgres until the worker runs |
| AWS Aurora / RDS for Postgres | Puts the DB in a VPC, so the Lambdas join the VPC and then need a NAT gateway to reach the LLM API, which costs more per month than the droplet |
| Postgres self-hosted on the droplet | Lambda has no fixed outbound IP, so 5432 would be open to the internet; backups become manual. Viable only if the worker writes results back through an app endpoint instead of the DB |
| DigitalOcean managed Postgres | Paid (from about $15/month) with the same Lambda access problem |
| Native iOS app now | Mainly buys offline queueing and widgets; that's iOS work, not AI work. Revisit if missed captures become frequent |
| Synchronous LLM call in ingest | The Shortcut runs at the payment terminal; ingest must be fast and never lose an event because the LLM is slow |
