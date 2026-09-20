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
│   └── seedadmin/main.go          # bootstrap: create the first admin account
├── migrations/0001_init_schema.sql
└── internal/
    ├── config/       # env var loading
    ├── database/     # pgxpool setup + health check
    ├── middleware/   # JWT auth + RBAC
    ├── models/       # domain types shared by every layer
    ├── pkg/notifier/ # email/SMS notification interface + log implementation
    ├── repository/   # ALL SQL lives here, fully parameterized
    ├── service/      # validation + business rules + pricing, no SQL, no HTTP
    └── handler/      # HTTP <-> JSON, routing (chi), no SQL
```

## Setup

1. **Postgres**: `createdb gemstore && psql -d gemstore -f schema.sql`
2. **Go deps**: `go mod tidy` (resolves `go.mod`, writes `go.sum`)
3. **Env vars**: `cp .env.example .env`, set `DATABASE_URL` and
   `JWT_SECRET` (`openssl rand -base64 32` — must be ≥32 characters,
   `config.Load` refuses to start without it)
4. **Run**: `export $(cat .env | xargs) && go run ./cmd/api`
5. **Bootstrap an admin** (public signup can never create one — see
   Security notes): `go run ./cmd/seedadmin -email owner@yourstore.com -password "..." -name "Store Owner"`

## API reference

| Method | Path                                  | Auth              | Notes |
|--------|----------------------------------------|--------------------|-------|
| GET    | `/health`                              | none               | DB connectivity check |
| GET    | `/api/v1/products`                     | none               | catalogue, `?category=&gender=` filters |
| GET    | `/api/v1/products/{id}`                | none               | |
| GET    | `/api/v1/pricing/catalog`              | none               | raw pricing tables, for client-side estimation |
| POST   | `/api/v1/auth/signup`                  | none               | always creates role `customer` |
| POST   | `/api/v1/auth/login`                   | none               | |
| GET    | `/api/v1/auth/me`                      | any logged-in user | |
| GET    | `/api/v1/orders`                       | any logged-in user | caller's own order history |
| POST   | `/api/v1/orders/custom`                | any logged-in user | server computes the authoritative price |
| GET    | `/api/v1/orders/{id}`                  | owner or admin      | 404 (not 403) if you don't own it |
| GET    | `/api/v1/admin/orders/pending`         | **admin**           | full customer + customization + shipping detail |
| POST   | `/api/v1/admin/orders/{id}/approve`    | **admin**           | `notes` optional |
| POST   | `/api/v1/admin/orders/{id}/decline`    | **admin**           | `notes` **required** |

## What's built

- **Schema & scaffold**: `users`, `products`, `customizations`, `orders`,
  `order_items`, `order_approvals` — layered Go backend, catalogue
  browsing, transactional custom-order submission.
- **Auth & RBAC**: JWT middleware (`internal/middleware/auth.go`, rejects
  non-HMAC algorithms), `RequireRole`, real signup/login with bcrypt
  (cost 12, timing-safe against email enumeration — see Security notes).
- **Approval workflow**: `POST .../approve` / `.../decline`, each a
  single DB transaction (status update + audit row in `order_approvals`
  + customer contact lookup), guarded against double-decision races by
  the `UPDATE ... WHERE status = 'pending_review'` pattern.
- **Notifications**: `notifier.NotificationService` interface
  (email/SMS), fired asynchronously after a decision commits, using its
  own `context.Background()` + timeout (never the request's context,
  which is cancelled the instant the handler returns).
- **Pricing engine**: `internal/service/pricing.go` — deterministic,
  catalog-driven, overrides whatever price a client submits. Exposed
  read-only via `GET /pricing/catalog` so a frontend can compute a
  matching estimate instead of hardcoding a second, driftable copy of
  the numbers.
- **Order history + item enrichment**: `GET /api/v1/orders` (added when
  the frontend needed it), with every `OrderItem` carrying its full
  `Customization`/`Product` detail inline rather than a bare ID.

## Verification

Every step was syntax-checked with `gofmt`; from the pricing engine
onward, `go build`/`go vet` ran clean using temporary module-mirror
redirects to work around this sandbox's restricted network (stripped
from the shipped `go.mod` — irrelevant on a machine with normal
internet, where `go mod tidy` resolves everything natively).

Beyond compiling, this was tested against **a real local Postgres 16
instance** with the actual compiled binary, across every major flow:

- signup (201/409/400 cases) · login (right/wrong password, `/auth/me`)
- customer blocked from admin routes (403) · admin approve/decline,
  including **double-approval correctly rejected with 409**
- async notifier **confirmed firing** via the server's own log line
- custom order pricing: a deliberately wrong client price was
  **overridden** by the server-computed total (verified against the
  hand-calculated expected value), with a drift warning logged
- unrecognized metal/gemstone → 400 with the supported list
- non-JSON `shipping_address` → 400; valid JSON → round-trips correctly
- `GET /api/v1/orders` returns multiple orders, most-recent-first, each
  with correctly nested customization detail and shipping address
  present/absent exactly where expected
- cross-customer order access → 404, not 403 (ownership masked)

## Assumptions / known gaps (flag anything you want changed)

1. **Public signup only creates customers** — no self-service path to
   admin. `cmd/seedadmin` bootstraps the first one; a "promote user"
   admin-only endpoint is a clean addition once there's an admin UI for it.
2. **No email verification** on signup — a token is usable immediately.
3. **No rate limiting on `/auth/login`** — timing-safe against
   enumeration, not yet throttled against brute-force guessing.
4. **Token TTL is 24h** (`JWT_TOKEN_TTL`), no refresh-token flow.
5. **Notifications are logged, not sent** — `LogNotifier` is the default;
   swap the one constructor call in `main.go` for a real provider
   (Resend/SMTP/SendGrid, Twilio/SNS) once you have credentials.
6. **Fire-and-forget notifications**, not a durable queue — no retry,
   won't survive a crash mid-send. A `notification_outbox` table +
   worker is the production upgrade path.
7. **HS256 shared-secret JWTs**, not RS256 — simplest for one service.
8. **Admin role is trusted from the JWT claim**, not re-checked against
   the DB at decision time — only matters if a token outlives a role
   downgrade before it expires.

## Security notes (cumulative)

- Every query in `internal/repository` is parameterized — no string
  concatenation of user input into SQL, anywhere, including the dynamic
  `WHERE` clause in `ProductRepository.List`.
- `DecideCustomOrder`'s status-guarded `UPDATE` is the concurrency
  control against two admins deciding the same order at once — verified
  against a real DB, not just reasoned about.
- JWT verification explicitly rejects any non-HMAC signing algorithm,
  closing the classic `"alg": "none"` bypass class.
- Passwords: bcrypt at cost 12 (above the library default of 10);
  `Login` takes materially the same time whether the email exists or
  not (a dummy bcrypt compare on the "no such user" path), closing a
  response-timing account-enumeration side channel.
- `ErrEmailTaken` / `ErrOrderNotPending` are both detected via DB-level
  guarantees (a unique-constraint error code; a status-guarded `UPDATE`
  matching zero rows) rather than a check-then-act — so the relevant
  race conditions can't happen no matter how two requests interleave.
- `GET /api/v1/orders/{id}` enforces ownership, masked as 404 rather
  than 403, so a non-owner can't distinguish "not yours" from "doesn't exist."
