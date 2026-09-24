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
6. **Real email/SMS (optional)**: set `SENDGRID_API_KEY` and/or
   `TWILIO_ACCOUNT_SID` + `TWILIO_AUTH_TOKEN` + `TWILIO_PHONE_NUMBER`.
   Leave any of them unset and that channel logs instead of sending —
   the app runs fully functional with zero provider credentials.
7. **Run the test suite**: `go test ./... -v` (the notifier package's
   tests need no network or credentials — see "Testing" below)

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

### Environment variables for notifications (all optional)

| Variable | Purpose |
|----------|---------|
| `SENDGRID_API_KEY` | enables real email via SendGrid; unset → logged, not sent |
| `EMAIL_FROM_ADDRESS`, `EMAIL_FROM_NAME` | sender identity — `EMAIL_FROM_ADDRESS` must be verified in your SendGrid account |
| `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_PHONE_NUMBER` | enables real SMS via Twilio; any missing → logged, not sent |

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
- **Real notification providers**: `SendGridEmailNotifier` and
  `TwilioSMSNotifier` (`internal/pkg/notifier/providers.go`) implement
  `NotificationService` against the real APIs, wrapped in a
  `CompositeNotifier` that lets email and SMS each independently fall
  back to `LogNotifier` when their credentials aren't set —
  `buildNotifier` in `main.go`. HTML + plain-text email templates
  (`internal/pkg/notifier/templates.go`) render via `html/template`
  (auto-escaping — customer engraving text and admin notes are
  untrusted input embedded in the HTML), with an item recap pulled from
  the enriched `Order.Items`.

## Testing

The notifier package has real `_test.go` files — the first tests in
this codebase, and a natural place for them to start, since this step's
code (HTTP request-building against providers, HTML template rendering
with untrusted input) is exactly the kind of thing worth pinning down
with tests rather than re-verifying by hand every time it changes.

```bash
go test ./internal/pkg/notifier/... -v
```

- **`providers_test.go`**: spins up local `httptest` servers shaped
  like SendGrid/Twilio's real APIs (this sandbox's network can't reach
  the real ones) and asserts the actual HTTP requests — method, path,
  auth header format, JSON/form body shape — are built correctly, plus
  that provider error responses surface a useful message and that long
  SMS bodies truncate safely (see below).
- **`templates_test.go`**: renders both templates and checks the
  expected content appears — and specifically asserts that an engraving
  text of `<script>alert(1)</script><img src=x onerror=alert(2)>` comes
  out HTML-escaped, not live. That's not a hypothetical: engraving text
  and admin notes are customer/admin-supplied strings embedded directly
  into an HTML email, so this is a real stored-HTML-injection surface
  if `html/template`'s auto-escaping were ever accidentally bypassed
  (e.g. by switching to `text/template` or manual string concatenation
  during a future edit).

**Two real bugs these tests caught before shipping** (both fixed, both
now covered by a regression test):

1. SMS truncation appended `"…"` (3 bytes in UTF-8, not 1) after
   slicing to `limit-1` bytes — overshot the byte cap by 2 and could
   slice a multi-byte character in half. Fixed in `truncateSMSBody`.
2. The order object built during approval/decline never had its
   `Items` populated (`DecideCustomOrder`'s `UPDATE ... RETURNING`
   only ever selected the order's own columns) — the confirmation
   email's item recap would have silently rendered empty. Fixed by
   fetching item detail after the transaction commits, same pattern as
   `OrderRepository.GetByID`.

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
- **This step**: `go test ./internal/pkg/notifier/...` — 11 tests, all
  passing (see "Testing" above) — plus a fresh end-to-end Postgres run
  confirming the real approval flow (submit → approve → notification)
  produces a correctly-populated email (customer name, item recap,
  admin notes) and SMS, visible in the server's own log output since no
  real SendGrid/Twilio credentials exist in this sandbox. **Actual
  delivery through the real SendGrid/Twilio APIs was never tested** —
  this sandbox's network allowlist doesn't include `api.sendgrid.com`
  or `api.twilio.com`. What was verified instead: the HTTP requests
  built for those APIs (method, path, headers, body shape) match their
  documented formats, checked via `httptest` mock servers standing in
  for the real ones. Worth a real smoke test with actual credentials
  before you rely on this in production — mock-server verification
  confirms the request is *shaped* correctly, not that a live account
  will actually accept and deliver it (e.g. an unverified sender
  address would be rejected by real SendGrid in a way no mock can catch).

## Assumptions / known gaps (flag anything you want changed)

1. **Public signup only creates customers** — no self-service path to
   admin. `cmd/seedadmin` bootstraps the first one; a "promote user"
   admin-only endpoint is a clean addition once there's an admin UI for it.
2. **No email verification** on signup — a token is usable immediately.
3. **No rate limiting on `/auth/login`** — timing-safe against
   enumeration, not yet throttled against brute-force guessing.
4. **Token TTL is 24h** (`JWT_TOKEN_TTL`), no refresh-token flow.
5. **SendGrid, not Resend** — the request named both as options; the
   explicit `SENDGRID_API_KEY` env var in the same request resolved
   that in SendGrid's favor. Swapping to Resend later means writing an
   equivalent `ResendEmailNotifier` against its (simpler) REST shape
   and changing one constructor call in `buildNotifier` — the
   `emailSender`/`NotificationService` interfaces don't change.
6. **Store name is hardcoded** (`storeName` in `templates.go`) — there's
   no branding/settings concept in the schema yet to source it from.
7. **Fire-and-forget notifications**, not a durable queue — no retry,
   won't survive a crash mid-send. A `notification_outbox` table +
   worker is the production upgrade path, more pressing now that real
   money-adjacent provider calls (not just logging) can actually fail.
8. **HS256 shared-secret JWTs**, not RS256 — simplest for one service.
9. **Admin role is trusted from the JWT claim**, not re-checked against
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
- Email templates use `html/template` (auto-escaping), not
  `text/template` or manual string concatenation — engraving text and
  admin notes are untrusted strings embedded directly into an HTML
  email, so unescaped interpolation there would be a real stored-HTML-
  injection vector. `templates_test.go` specifically asserts a
  `<script>`/`onerror=` payload in engraving text renders inert.
