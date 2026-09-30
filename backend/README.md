# Gem & Jewelry Store — Backend

Go + PostgreSQL backend for the Gem & Jewelry Store Management System.
Layered architecture: `handler -> service -> repository -> pgxpool`.

## Layout

```
gem-store-backend/
├── go.mod
├── schema.sql                          # full current schema — run this on a fresh DB
├── .env.example
├── cmd/
│   ├── api/main.go                     # server entry point, graceful shutdown
│   ├── gentoken/main.go                # DEV: mints a test JWT without going through login
│   ├── seedadmin/main.go               # bootstrap: create the first admin account
│   └── seedproducts/main.go            # bulk-insert the ready-made catalogue from JSON
├── seed/products.json                  # sample catalogue data — see "Product images"
├── migrations/
│   ├── 0001_init_schema.sql
│   └── 0002_refresh_tokens.sql         # cookie/refresh-token auth (this step)
└── internal/
    ├── config/       # env var loading
    ├── database/     # pgxpool setup + health check
    ├── middleware/   # auth/RBAC, rate limiting, body-size limits, security headers, panic recovery
    ├── models/       # domain types shared by every layer
    ├── pkg/notifier/ # email/SMS notification interface + real providers + log fallback
    ├── repository/   # ALL SQL lives here, fully parameterized
    ├── service/      # validation + business rules + pricing + auth, no SQL, no HTTP
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
6. **Seed the ready-made catalogue** (empty by default — nothing creates
   products otherwise): `go run ./cmd/seedproducts -file seed/products.json`.
   Re-running is safe — existing SKUs are skipped, not duplicated or
   errored on. See "Product images" below for wiring in real photos.
7. **Real email/SMS (optional)**: set `SENDGRID_API_KEY` and/or
   `TWILIO_ACCOUNT_SID` + `TWILIO_AUTH_TOKEN` + `TWILIO_PHONE_NUMBER`.
   Leave any of them unset and that channel logs instead of sending —
   the app runs fully functional with zero provider credentials.
8. **Run the test suite**: `go test ./... -v` — 19 tests, no network or
   DB required (see "Testing" below).

## Product images

`products.image_url` is just a string column — the backend stores
whatever path or URL you give it and never fetches, validates, or
processes it. There's no admin UI for product management yet, so
`cmd/seedproducts` (bulk insert from `seed/products.json`) is the only
way products get into the table at all.

**Use a path your frontend actually serves, not a hotlinked third-party
URL** — e.g. `/images/products/ring-001.jpg` for a file placed in the
Next.js app's `public/images/products/` directory. `seed/products.json`
ships with exactly these paths as placeholders; the images themselves
aren't included (real product photography is presumably licensed/owned
by whoever runs the actual store, so it isn't something to bundle
sight-unseen into a coursework deliverable). Until real files exist at
those paths, the frontend's `ProductImage` component falls back to a
plain placeholder rather than a broken-image icon — see the frontend
README.

## API reference

| Method | Path                                  | Auth              | Notes |
|--------|----------------------------------------|--------------------|-------|
| GET    | `/health`                              | none               | DB connectivity check |
| GET    | `/api/v1/products`                     | none               | catalogue, `?category=&gender=` filters — try it after `cmd/seedproducts` |
| GET    | `/api/v1/products/{id}`                | none               | |
| GET    | `/api/v1/pricing/catalog`              | none               | raw pricing tables, for client-side estimation |
| POST   | `/api/v1/auth/signup`                  | none, **rate-limited** | always creates role `customer`; sets auth cookies |
| POST   | `/api/v1/auth/login`                   | none, **rate-limited** | sets auth cookies |
| POST   | `/api/v1/auth/refresh`                 | refresh cookie      | rotates both tokens; see "Auth model" below |
| POST   | `/api/v1/auth/logout`                  | none                | revokes + clears cookies; safe with no session |
| GET    | `/api/v1/auth/me`                      | any logged-in user | |
| GET    | `/api/v1/orders`                       | any logged-in user | caller's own order history |
| POST   | `/api/v1/orders/custom`                | any logged-in user | server computes the authoritative price |
| GET    | `/api/v1/orders/{id}`                  | owner or admin      | 404 (not 403) if you don't own it |
| GET    | `/api/v1/admin/orders/pending`         | **admin**           | full customer + customization + shipping detail |
| POST   | `/api/v1/admin/orders/{id}/approve`    | **admin**           | `notes` optional |
| POST   | `/api/v1/admin/orders/{id}/decline`    | **admin**           | `notes` **required** |

"Rate-limited" = 5 attempts/minute/IP, 429 beyond that — see "Security hardening" below.

### Environment variables

| Variable | Purpose |
|----------|---------|
| `DATABASE_URL`, `JWT_SECRET` | **required** |
| `ACCESS_TOKEN_TTL` (default `15m`), `REFRESH_TOKEN_TTL` (default `168h` / 7 days) | token lifetimes — see "Auth model" |
| `FRONTEND_ORIGIN` (default `http://localhost:3000`) | the *only* origin CORS accepts, and it must be exact (not a wildcard) for credentialed cookie requests to work at all |
| `SENDGRID_API_KEY`, `EMAIL_FROM_ADDRESS`, `EMAIL_FROM_NAME` | real email; unset → logged, not sent |
| `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_PHONE_NUMBER` | real SMS; any missing → logged, not sent |

## Auth model: HttpOnly cookies + short-lived access + rotating refresh

Login/signup/refresh set two cookies — never a token in the JSON body,
since anything a `fetch()` response returns is readable by injected XSS
JS just as easily as by your own code, which would defeat the point of
HttpOnly:

- **`access_token`** — a JWT, 15 minutes by default, `Path=/`, sent on
  every request. Stateless — cannot be revoked early, only allowed to
  expire, which is exactly why it's short-lived.
- **`refresh_token`** — a random 256-bit value, 7 days by default,
  `Path=/api/v1/auth` (never sent anywhere else), backed by a
  `refresh_tokens` DB row storing only its SHA-256 hash. **Rotates on
  every use**: `POST /auth/refresh` issues a new access+refresh pair and
  immediately revokes the refresh token that was just presented. A
  client only ever presents a given refresh token once by construction
  — so if an already-revoked one is presented again, that's treated as
  a signal of possible theft, and *every* refresh token for that user is
  revoked immediately, forcing every session to re-authenticate.
  Verified live: replaying an old token after rotation correctly fails,
  and — critically — the token that *was* still valid a moment earlier
  is also dead afterward.

Every refresh re-fetches the user fresh from the DB rather than trusting
anything cached, which also shrinks how long a role change (promotion,
demotion, ban) can go unnoticed: at most `ACCESS_TOKEN_TTL`, not
"whenever they happen to log in again."

Both cookies are `HttpOnly; Secure; SameSite=Strict`. Two things worth
knowing about that:

- **`Secure` doesn't break local `http://localhost` dev** — modern
  browsers special-case `localhost` as a trustworthy origin for this
  flag even without TLS. Any other non-HTTPS origin would silently drop
  these cookies, which is correct in production.
- **`SameSite=Strict` requires the frontend and backend to share a
  registrable domain** (subdomains/ports of the same site — which is
  exactly `localhost:3000` ↔ `localhost:8080`, or `app.yourstore.com` ↔
  `api.yourstore.com`). It's also *why* this is a strong CSRF defense on
  its own: a cross-site request literally never carries these cookies.
  If you ever deploy the frontend and backend on genuinely unrelated
  domains, `SameSite=Strict` will silently block them on every call —
  you'd need `SameSite=None; Secure` instead, which reopens the CSRF
  surface `Strict` closes and should come with an explicit CSRF-token
  scheme, not a bare flag flip.

`Authenticate` (middleware) reads the access token from the cookie
first, falling back to an `Authorization: Bearer` header — kept for
non-browser clients and `cmd/gentoken`-minted test tokens, which have no
way to arrive as a cookie.

## Security hardening (this step)

- **Rate limiting** (`internal/middleware/security.go`): a
  `golang.org/x/time/rate` token bucket per client IP, 5/min, applied
  only to `/auth/login` and `/auth/signup` (separate limiter instances,
  so one doesn't count against the other). Stale IP entries are swept
  every 10 minutes so the map doesn't grow forever. **Deployment
  caveat**: this keys on `r.RemoteAddr`, which chi's `middleware.RealIP`
  rewrites from `X-Forwarded-For`/`X-Real-IP` — trustworthy only behind
  a real reverse proxy that sets those headers itself; if this server
  is ever directly internet-facing, a client can spoof those headers
  and get a fresh bucket on every request, bypassing the limit entirely.
- **Body size limit**: every request body capped at 1 MiB
  (`appmiddleware.MaxBodySize`, applied globally) via
  `http.MaxBytesReader`. `httputil.DecodeJSON` — now used by the auth
  handlers — detects the resulting `*http.MaxBytesError` specifically
  and returns 413 with a clear message rather than a generic 400.
  (`order_handler.go`/`admin.go` still do their own inline decode +
  generic-400 handling; migrating them to `DecodeJSON` too is a small,
  easy follow-up, not done here to keep this step's diff focused on auth.)
- **Security headers** (`internal/middleware/headers.go`), on every
  response: `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`,
  `Referrer-Policy: strict-origin-when-cross-origin`,
  `Strict-Transport-Security` (inert over plain HTTP, harmless to always
  send), and a locked-down `Content-Security-Policy: default-src 'none'`
  — deliberately *not* a typical webpage CSP with script-src/style-src
  allow-lists, because this is a JSON API that never returns HTML/JS for
  a browser to execute in the first place.
- **Panic recovery** (`internal/middleware/recovery.go`) replaces chi's
  `middleware.Recoverer`: logs the panic value and full stack trace via
  structured `slog` (matching the rest of the app's logging, unlike
  chi's colorized-stdout default) and returns the same generic
  `{"error": "internal server error"}` JSON shape every other error path
  uses — never the panic value, a stack trace, or a file path to the
  client. Verified with a handler that panics with a string containing a
  fake DB password and an internal file path: the client response
  contains neither, confirmed by asserting on the exact strings.
- **Strict CORS**: `AllowedOrigins` is the single configured
  `FRONTEND_ORIGIN`, never a wildcard — and couldn't be anyway, since
  browsers refuse credentialed (cookie-carrying) requests against a
  wildcard origin regardless of server config.

### Testing

```bash
go test ./... -v
```

19 tests across two packages, all passing, no network or DB required:

- **`internal/pkg/notifier`** (11, from the previous step): provider
  request-shape verification against `httptest` mocks, email template
  rendering + HTML-escaping of injected script tags, SMS truncation.
- **`internal/middleware`** (7, this step): rate limiter burst behavior
  and per-IP isolation, the 429 response, `MaxBodySize` rejecting an
  oversized body, and panic recovery's generic-response + no-leak guarantee.
- **`internal/httputil`** (4, this step): `DecodeJSON`'s success,
  empty-body, malformed-JSON, and oversized-body (413) cases.

**A real bug these tests caught before shipping**: the original SMS
truncation appended `"…"` (3 bytes in UTF-8) after slicing to
`limit-1` bytes — overshooting the byte cap by 2 and risking a
mid-rune cut. `truncateSMSBody` now backs off to a valid rune boundary
before appending. (From the previous step; still the most concrete
example of why these tests exist.)

## Verification

Every step was syntax-checked with `gofmt`; `go build`/`go vet` run
clean using temporary module-mirror redirects to work around this
sandbox's restricted network (stripped from the shipped `go.mod` —
irrelevant on a machine with normal internet, where `go mod tidy`
resolves everything natively).

Beyond compiling and the unit test suite above, **this step was tested
against a real local Postgres 16 instance with the actual compiled
binary**, using `curl` with a real cookie jar (not manually copied
tokens) end to end:

- Signup: `Set-Cookie` headers inspected directly and confirmed
  `HttpOnly; Secure; SameSite=Strict` on both cookies, correct `Path`
  (`/` vs `/api/v1/auth`) and `Max-Age` matching the configured TTLs;
  response body confirmed to contain **no** token field
- `GET /api/v1/orders` and `/auth/me` succeed via the cookie alone, no
  `Authorization` header sent
- Access token allowed to actually expire (ran with `ACCESS_TOKEN_TTL=5s`
  for this), then confirmed rejected; `POST /auth/refresh` then issues a
  working new pair, confirmed by a subsequent successful `/auth/me`
- **Reuse-of-a-rotated-token attack simulated for real**: captured the
  refresh token before rotating it, rotated (via a legitimate refresh),
  then replayed the old one — rejected, *and* the token that had just
  been legitimately issued by that same rotation was also dead
  afterward, confirming mass revocation actually fires (and, once a
  logging gap found during this exact test was fixed, is now logged as
  a security event)
- Logout: revokes server-side and clears cookies; confirmed `/auth/me`
  fails afterward
- Rate limiting: 6 rapid failed logins from one IP → attempts 1-5 return
  401, the 6th returns 429
- All 5 security headers confirmed present with exact expected values
  on a normal response
- A 2 MB signup payload against the 1 MiB cap → 413 with the correct message
- **This step**: `cmd/seedproducts` run against a fresh DB with the
  sample `seed/products.json` — all 6 products created; re-run
  confirmed as a no-op (existing SKUs skipped, not duplicated or
  errored); verified via the real `GET /api/v1/products` endpoint
  including `?category=&gender=` filtering, not just by inspecting the DB directly

## Assumptions / known gaps (flag anything you want changed)

1. **Public signup only creates customers** — no self-service path to
   admin. `cmd/seedadmin` bootstraps the first one.
2. **No email verification** on signup — a session is usable immediately.
3. **SendGrid, not Resend** — both were named as options; the explicit
   `SENDGRID_API_KEY` env var resolved that. Swapping later is a new
   `ResendEmailNotifier` implementation plus one constructor-call change.
4. **Store name is hardcoded** (`storeName` in `templates.go`) — no
   branding/settings concept in the schema yet.
5. **Fire-and-forget notifications**, not a durable queue — no retry,
   won't survive a crash mid-send.
6. **HS256 shared-secret JWTs**, not RS256 — simplest for one service.
7. **No refresh-token cleanup job** — expired/revoked rows accumulate in
   `refresh_tokens` indefinitely. A periodic `DELETE ... WHERE
   expires_at < now() OR revoked_at IS NOT NULL` (a cron job, or a
   `pg_cron` extension) is the natural addition; `idx_refresh_tokens_expires_at`
   already exists to make that query cheap.
8. **`order_handler.go`/`admin.go` don't yet use the new
   `httputil.DecodeJSON` helper** the auth handlers do — same behavior,
   just a less precise error message on an oversized body (generic 400
   instead of 413). Small, easy follow-up.
9. **Admin role is now re-checked on every refresh** (at most
   `ACCESS_TOKEN_TTL` staleness), a real improvement over the previous
   "trusted from the JWT claim until it expires, whenever that was" —
   but still not instant; a role change takes up to `ACCESS_TOKEN_TTL`
   to take effect for someone mid-session.
10. **No purchase flow for ready-made products.** The proposal calls for
    a direct "Buy Now" path for standard (non-customized) items
    (Section 5.1) — only custom orders (`POST /orders/custom`) actually
    work end to end. `products`/`ProductRepository.Create` exist now
    (this step, for the catalogue), but nothing creates a
    `standard`-type order from one. The frontend's new catalogue page
    is honest about this — it links to `/customize` rather than a
    non-functional "Buy Now" button. A real implementation needs a cart/
    checkout flow and a payment-provider decision neither of which has
    been discussed yet.
11. **No admin UI for product management** — `cmd/seedproducts` (bulk
    JSON insert) is the only way products get into the catalogue.

## Security notes (cumulative)

- Every query in `internal/repository` is parameterized — no string
  concatenation of user input into SQL, anywhere.
- `DecideCustomOrder`'s status-guarded `UPDATE`, refresh-token rotation,
  and reuse detection are all DB-level guarantees against race
  conditions (a status-guarded `UPDATE` matching zero rows; a unique
  constraint; revoke-then-check) rather than check-then-act — verified
  against a real DB under real concurrent/replayed requests, not just
  reasoned about.
- JWT verification explicitly rejects any non-HMAC signing algorithm,
  closing the classic `"alg": "none"` bypass class.
- Passwords: bcrypt at cost 12; `Login` takes materially the same time
  whether the email exists or not, closing a response-timing
  account-enumeration side channel. Refresh tokens use SHA-256, not
  bcrypt — deliberately: a refresh token is already 256 bits of random
  data, not a human-memorable secret, so there's nothing for bcrypt's
  slowness to defend against; a fast hash is the right tool.
- `GET /api/v1/orders/{id}` enforces ownership, masked as 404 rather
  than 403.
- Email templates use `html/template` (auto-escaping) — engraving text
  and admin notes are untrusted strings embedded directly into HTML;
  `templates_test.go` asserts an injected `<script>` tag renders inert.
- Panic recovery never leaks a stack trace, error detail, or file path
  to the client — verified with a test asserting on the exact absence
  of planted sensitive strings in the response body.
- CORS accepts exactly one configured origin, never a wildcard; combined
  with `SameSite=Strict` cookies, this is the actual CSRF defense for
  this deployment topology (frontend + backend sharing a registrable
  domain) — see "Auth model" above for the caveat if that ever changes.
