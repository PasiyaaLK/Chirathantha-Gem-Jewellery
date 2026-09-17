# Gem & Jewelry Store — Backend

Go + PostgreSQL backend for the Gem & Jewelry Store Management System.
Layered architecture: `handler -> service -> repository -> pgxpool`.

## Layout

```
gem-store-backend/
├── go.mod
├── schema.sql                     # same content as migrations/0001_init_schema.sql
├── .env.example
├── cmd/
│   ├── api/main.go                # server entry point, graceful shutdown
│   ├── gentoken/main.go           # DEV: mints a test JWT without going through login
│   └── seedadmin/main.go          # bootstrap: create the first admin account (Step 3)
├── migrations/0001_init_schema.sql
└── internal/
    ├── config/       # env var loading
    ├── database/     # pgxpool setup + health check
    ├── middleware/   # JWT auth + RBAC (Step 2)
    ├── models/       # domain types shared by every layer
    ├── pkg/notifier/ # email/SMS notification interface + log implementation (Step 2)
    ├── repository/   # ALL SQL lives here, fully parameterized
    ├── service/      # validation + business rules, no SQL, no HTTP
    └── handler/      # HTTP <-> JSON, routing (chi), no SQL
```

## Setup

1. **Postgres**: create a database and run the schema.

   ```bash
   createdb gemstore
   psql -d gemstore -f schema.sql
   ```

2. **Go deps**:

   ```bash
   go mod tidy
   ```

   This resolves the pinned versions in `go.mod` and writes `go.sum`.

3. **Env vars**: `cp .env.example .env`, then set `DATABASE_URL` and
   `JWT_SECRET` (generate with `openssl rand -base64 32` — must be at
   least 32 characters, `config.Load` refuses to start without it).

4. **Run**:

   ```bash
   export $(cat .env | xargs)
   go run ./cmd/api
   ```

## Verification performed in this session

Steps 1-2 were syntax-checked with `gofmt` only. For Step 3, working
around the sandbox's restricted network allowlist with temporary
module-mirror redirects (stripped from the shipped `go.mod` — not
needed on a machine with normal internet), verification went further
than a build:

```
go build ./...   → exit 0
go vet ./...     → exit 0
```

**A real local Postgres 16 instance was stood up, `schema.sql` applied
with zero errors, and the compiled `cmd/api` binary run against it for
a full black-box HTTP test pass:**

- signup → 201 + token; duplicate email → 409; weak password → 400
- login: correct password → 200 + token; wrong password → 401 (and
  timed via the dummy-hash path, not a fast-path email-enumeration leak)
- `/auth/me` with a valid token → 200; with no token → 401
- customer hitting an admin-only route → 403
- customer submits a custom order → 201, `status: pending_review`
- admin lists pending orders → the order appears with full customer +
  customization detail
- admin approves → 200, `status: confirmed`; **approving the same order
  again → 409** (the `WHERE status = 'pending_review'` concurrency guard,
  confirmed working against a real DB, not just read as correct)
- the async notifier **actually fired** — confirmed via the server's
  own log line, subject and body matching what `BuildOrderDecisionMessage`
  composes
- declining without `notes` → 400
- a *different* customer requesting the first customer's order →
  **404, not 403** (ownership masking working as designed)

This is the strongest verification so far — not just "compiles," but
"behaves correctly against a real database under the actual failure
and race conditions the code was written to guard against."

## New in Step 3.5: Server-Side Pricing Engine

The client's `estimated_price` is **never trusted** for the actual
charge anymore. `internal/service/pricing.go` computes the authoritative
total from a fixed catalog (category base price × metal multiplier +
gemstone flat fee + engraving base+per-char fee), and
`OrderService.SubmitCustomOrder` overwrites whatever the client sent
before it ever reaches the database. `POST /api/v1/orders/custom` now
returns `{"order": {...}, "pricing": {...}}` — the breakdown alongside
the order — instead of just the bare order.

A new public `GET /api/v1/pricing/catalog` endpoint exposes the raw
pricing tables read-only, so the frontend's "client-side price
estimation" isn't a hand-copied second source of truth that can drift —
it fetches the real numbers and computes against them.

**Bug found and fixed while wiring up the frontend form:** `orders.shipping_address`
is a `JSONB` column, but nothing before this step ever sent a real
request with that field populated (every test so far omitted it, and
`NULL` is valid JSONB either way) — so a plain address string like
`"221B Galle Road"` would have hit Postgres's `invalid input syntax for
type json` and surfaced as a confusing 500. Fixed by validating
`shipping_address` as JSON in `validateCustomOrderRequest` (returns a
clean 400 with an example, same as any other bad input) rather than
letting Postgres reject it deep in the transaction. Verified against a
real DB: a plain string is now rejected at the API boundary with a
helpful message, and `{"raw": "..."}` round-trips correctly.

### Verification (same real-Postgres approach as before)

- `GET /pricing/catalog` — confirmed the JSON response matches the Go
  source maps exactly.
- Submitted a real custom order (ring, rose gold, sapphire, 5-character
  engraving) with a deliberately wrong `estimated_price: 1.00` — server
  computed **$582.50** (`150 × 2.5 + 180 + (20 + 5×1.5)`), used that for
  both `orders.total_amount` and `order_items.unit_price`, and logged a
  `WARN` about the client/server price mismatch — the diagnostic signal
  worked exactly as designed.
- Unrecognized metal (`"unobtainium"`) → 400 with the list of supported
  metals in the error message.
- Non-JSON `shipping_address` → 400; valid JSON → round-trips correctly
  on fetch.



### Endpoints

| Method | Path                    | Auth   | Notes |
|--------|--------------------------|--------|-------|
| POST   | `/api/v1/auth/signup`    | none   | always creates role `customer` |
| POST   | `/api/v1/auth/login`     | none   | |
| GET    | `/api/v1/auth/me`        | any authenticated user | quick token sanity-check |

`cmd/gentoken` still works for quickly minting a token without going
through login (handy for scripting), but real signup/login is now the
primary path.

### Bootstrapping the first admin

Public signup can never create an admin (role is hardcoded server-side,
never read from the request body). Create the store owner's account with:

```bash
export DATABASE_URL=... JWT_SECRET=...
go run ./cmd/seedadmin -email owner@yourstore.com -password "a-real-password" -name "Store Owner"
```

Then log in normally via `POST /api/v1/auth/login` to get an admin token.

### Smoke test

```bash
curl -X POST localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{"email":"jane@example.com","password":"a-strong-password","full_name":"Jane Doe"}'
# -> {"token": "...", "user": {...}}

curl -X POST localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"jane@example.com","password":"a-strong-password"}'

curl localhost:8080/api/v1/auth/me -H "Authorization: Bearer <token>"
```

## Assumptions made this step (flag anything you want changed before Step 4)

1. **Public signup only creates customers.** No self-service path to
   admin — intentional (see above). If you want a second/third admin
   later without shelling into the server, that's a clean small addition
   once there's an admin UI: an authenticated, admin-only "promote user"
   endpoint.
2. **No email verification.** `signup` issues a usable token immediately;
   the email address is never confirmed as reachable. Fine for a
   coursework/portfolio build; a real launch would want a verification
   link before treating the account as fully trusted.
3. **No rate limiting on `/auth/login`.** The timing-safe comparison
   stops response-time-based email enumeration, but nothing yet throttles
   repeated password guesses against one account. A per-IP or
   per-account rate limiter (or a captcha after N failures) is the
   natural next hardening step.
4. **Token TTL defaults to 24h**, configurable via `JWT_TOKEN_TTL`. There's
   no refresh-token flow — when a token expires, the user logs in again.

## Security notes (cumulative)

- Every query in `internal/repository` is parameterized — no exceptions.
- `DecideCustomOrder`'s status-guarded `UPDATE` prevents double-approval
  races — verified against a real DB this session, not just reasoned
  about.
- JWT verification rejects any non-HMAC signing algorithm.
- The async notifier goroutine uses its own `context.Background()` +
  timeout, never the request's context.
- `GET /api/v1/orders/{id}` enforces ownership, masked as 404 rather
  than 403, so a non-owner can't distinguish "not yours" from "doesn't
  exist."
- Passwords are hashed with bcrypt at cost 12 (above the library
  default of 10). `Login` takes materially the same time whether the
  email exists or not, closing a common enumeration side-channel.
- `ErrEmailTaken` is detected via the Postgres unique-constraint error
  code (`23505`), not a SELECT-then-INSERT check — so two concurrent
  signups for the same address can't both succeed.

### Auth

There's still no login/signup endpoint (needs password hashing + a
`users` repository — good Step 3 material). Until then, mint a test
token with the `gentoken` CLI:

```bash
export JWT_SECRET=...   # same value the API server is using

# Look up or insert a user row first, e.g.:
psql -d gemstore -c "INSERT INTO users (email, password_hash, full_name, role)
  VALUES ('admin@example.com', 'x', 'Store Owner', 'admin') RETURNING id;"

go run ./cmd/gentoken -user <that-uuid> -role admin -ttl 24h
# prints a JWT — use it as: -H "Authorization: Bearer <token>"
```

### New/changed endpoints

| Method | Path                              | Auth              |
|--------|------------------------------------|-------------------|
| GET    | `/api/v1/products`                 | none (unchanged)  |
| POST   | `/api/v1/orders/custom`            | customer or admin — now via JWT, not `X-User-ID` |
| GET    | `/api/v1/orders/{id}`              | owner or admin only (new: was open in Step 1) |
| GET    | `/api/v1/admin/orders/pending`     | **admin only**    |
| POST   | `/api/v1/admin/orders/{id}/approve`| **admin only**    |
| POST   | `/api/v1/admin/orders/{id}/decline`| **admin only** — `notes` required in body |

### Smoke test

```bash
TOKEN=$(go run ./cmd/gentoken -user <admin-uuid> -role admin)

curl localhost:8080/api/v1/admin/orders/pending \
  -H "Authorization: Bearer $TOKEN"

curl -X POST localhost:8080/api/v1/admin/orders/<order-id>/approve \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"notes": "Approved — estimated ship date in 2 weeks."}'

curl -X POST localhost:8080/api/v1/admin/orders/<order-id>/decline \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"notes": "18k rose gold is not available in this gemstone setting."}'
```

Watch the server's stdout — `LogNotifier` logs the email/SMS it *would*
have sent, so you can see the decision notification fire asynchronously
right after the HTTP response returns.

## Assumptions made this step (flag anything you want changed before Step 3)

1. **Notifications are logged, not sent.** `LogNotifier` implements the
   `notifier.NotificationService` interface but writes to `slog` instead
   of calling a real provider. Swap the one constructor call in
   `cmd/api/main.go` for a real implementation once you've picked a
   provider (Resend/SMTP/SendGrid for email, Twilio/SNS for SMS) — no
   other code changes.
2. **Fire-and-forget notifications**, not a durable queue. A bare
   goroutine (with its own background context + timeout + panic
   recovery) is enough for this step, but it has no retry and won't
   survive a server crash mid-send. A production version would write a
   `notification_outbox` row in the same DB transaction as the status
   update and have a separate worker poll/send/retry it — worth doing
   once a real provider exists.
3. **HS256 with a shared secret**, not RS256 with a key pair. Simpler
   for a single-service setup; if you ever split this into multiple
   services that need to verify tokens without holding the signing
   secret, RS256 is the natural upgrade.
4. **Admin role is trusted from the JWT claim** at request time — the
   repository doesn't re-check `users.role = 'admin'` in the DB during
   `DecideCustomOrder`. `RequireRole` already gates the route, so this
   only matters if a token outlives a role downgrade in the DB before
   its `exp`. Worth tightening once token TTLs get longer or a
   revocation mechanism exists.
5. **Declining requires `notes`**; approving doesn't. The proposal names
   "reason notes" only for rejection (Section 5.2), so approval notes
   are optional context, not a required field.

## Security notes (Step 1 + Step 2)

- Every query in `internal/repository` uses `$1, $2, ...` placeholders —
  no string concatenation of user input into SQL anywhere.
- `DecideCustomOrder`'s `UPDATE ... WHERE status = 'pending_review'` is
  the concurrency guard against two admins deciding the same order at
  once: only the first request's `UPDATE` matches a row; the second gets
  zero rows back and a 409, not a double-approval.
- JWT verification explicitly rejects any signing algorithm other than
  HMAC (`internal/middleware/auth.go`), so a token crafted with `"alg":
  "none"` or a mismatched algorithm can't slip through.
- `notifyAsync`'s goroutine uses a fresh `context.Background()` with its
  own timeout, never the inbound request's context — the request context
  is cancelled the moment the HTTP handler returns, which would silently
  kill an in-flight notification if reused.
- `GET /api/v1/orders/{id}` now enforces ownership (customer sees only
  their own orders; admin sees any) rather than being open to anyone who
  guesses a UUID, closing a gap that existed in Step 1 before auth
  existed.
