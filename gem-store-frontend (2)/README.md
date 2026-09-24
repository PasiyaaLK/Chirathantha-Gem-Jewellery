# Gem & Jewelry Store — Frontend pieces

Meant to be dropped into an existing Next.js 14+ (App Router) +
TypeScript project at these exact relative paths:

```
lib/types.ts
lib/api.ts
lib/pricing.ts
lib/styles.ts
lib/format.ts
lib/useAuth.ts
components/Visualizer.tsx
app/customize/page.tsx
app/login/page.tsx
app/signup/page.tsx
app/orders/page.tsx
app/admin/dashboard/page.tsx
```

## Prerequisites / assumptions

1. **Next.js App Router**, not Pages Router.
2. **The `@/*` import alias** resolving to your project root (the
   `create-next-app` + TypeScript default). Adjust imports if yours differs.
3. **Tailwind CSS**, via arbitrary hex values (`bg-[#161310]`) rather
   than theme tokens, so these work regardless of your `tailwind.config`.
4. **`three`**: `npm install three` + `npm install -D @types/three`.
5. **`NEXT_PUBLIC_API_BASE_URL`** in `.env.local` — defaults to
   `http://localhost:8080`, matching the backend's default port and its
   `FRONTEND_ORIGIN` CORS default of `http://localhost:3000`, so local
   dev works with zero config on either side.

## Auth model: HttpOnly cookies, not localStorage

**This changed from earlier versions of this frontend.** Auth used to
work via a token returned in the JSON response, stored in
`localStorage`, and manually attached as an `Authorization` header. The
backend now sets the access/refresh token pair as `HttpOnly; Secure;
SameSite=Strict` cookies instead — see the backend README's "Auth
model" section for the full design (short-lived access token, rotating
revocable refresh token, reuse-detection).

What that means here:

- **No more `getToken()`/`setToken()`/`clearToken()`.** An HttpOnly
  cookie is invisible to JavaScript by design — there's nothing left for
  client code to read or store. `lib/api.ts` now sends
  `credentials: "include"` on every request instead, which tells the
  browser to attach the cookies automatically.
- **`lib/useAuth.ts`'s `useCurrentUser()`** no longer checks for a
  stored token before deciding whether to call `/auth/me` — it always
  calls it, and treats any failure as "not logged in." That's the only
  way to know anymore.
- **Login/signup pages** no longer call `setToken(result.token)` —
  there is no token in the response body at all
  (`models.AuthResponse` is now just `{ user }`). The cookies are set
  directly by the response's `Set-Cookie` headers.
- **Silent access-token refresh**: `apiFetch` in `lib/api.ts` catches a
  401 on any non-auth endpoint, attempts one `POST /api/v1/auth/refresh`
  behind the scenes, and retries the original request once if that
  succeeds. With a 15-minute access token, this is what makes the app
  usable at all — without it, everyone would be silently logged out
  every 15 minutes. Concurrent 401s (e.g. a page firing off several
  requests at once right as the token expires) share one in-flight
  refresh rather than each racing to refresh independently — see the
  comment on `refreshInFlight` for why that matters given the backend
  rotates the refresh token on every use.
- **`logout()`** (new, in `lib/api.ts`) calls `POST /api/v1/auth/logout`,
  which revokes the session server-side and clears both cookies. This
  is now the *only* way to log out — wired into small "Log out" buttons
  on the admin dashboard and orders page.
- **`app/customize/page.tsx`** no longer has a client-side
  `if (!getToken())` pre-check before submitting — there's nothing
  client-side left to check. It just submits, and a 401 (not logged in)
  surfaces through the normal `ApiError` catch block like any other
  server-side validation failure.

## What each file does

- **`lib/types.ts`** — TypeScript mirrors of the Go backend's JSON
  shapes. `AuthResponse` is now `{ user: User }` — no token field.
- **`lib/pricing.ts`** — fetches `GET /api/v1/pricing/catalog` and
  computes a price estimate from the real numbers rather than a
  hardcoded, driftable copy.
- **`lib/api.ts`** — cookie-based `fetch` wrapper: `credentials:
  "include"` on every call, the silent-refresh-on-401 interceptor
  described above, `logout()`, and `ApiError` parsing of the backend's
  `{"error": "..."}` shape.
- **`components/Visualizer.tsx`** — Three.js 3D preview (procedural
  geometry — no jewelry asset pipeline exists) with material driven by
  the selected metal, a small mesh for the gemstone, and a live price badge.
- **`app/customize/page.tsx`** — the full customization form, wired to
  `Visualizer` and to `POST /api/v1/orders/custom`.
- **`lib/styles.ts`** — shared Tailwind class strings and the
  `STATUS_BADGE` map, so pages share one visual language.
- **`lib/format.ts`** — `titleCase`, `formatDate`, `formatCurrency`,
  `formatShippingAddress`.
- **`lib/useAuth.ts`** — `useCurrentUser()`; see "Auth model" above.
- **`app/login/page.tsx`** / **`app/signup/page.tsx`** — forms calling
  the real backend routes (`/api/v1/auth/login` /
  `/api/v1/auth/signup` — the request that first asked for these named
  `/register`, which doesn't exist; corrected), routing by role after
  login (admin → `/admin/dashboard`, customer → `/orders`).
- **`app/admin/dashboard/page.tsx`** — role-protected via
  `useCurrentUser`; pending-orders queue; an inspection drawer with
  full customization/shipping detail and Approve/Decline actions; a
  "Log out" button.
- **`app/orders/page.tsx`** — the customer's own order history with
  per-order status badges; a "Log out" button.

## Backend changes required to support these pages (cumulative)

1. **`GET /api/v1/orders`** didn't exist until it was added — there was
   only single-order lookup by ID, nothing for an order *history* list.
2. **Order items now carry full nested `Customization`/`Product`
   detail**, not just a bare ID — needed to render anything meaningful
   ("Ring — Rose Gold, Sapphire").
3. **`shipping_address`** was missing from the admin pending-orders
   response — added, since the inspection drawer needs it.
4. **Cookie-based auth + refresh/logout endpoints** (this step) — see
   the backend README's "Auth model" section. If you're pointing this
   frontend at an older backend copy that predates this, login/signup
   will still return a JSON `token` field these pages no longer read,
   and nothing will actually authenticate.

## Verification performed

Every version of this frontend has been type-checked for real, not
eyeballed: a scratch npm project with the **real** `next`, `react`, and
`three` packages (plus real `@types/*`) at realistic versions, a
standard Next.js `tsconfig.json` with the `@/*` alias configured, and
`tsc --noEmit --strict` run across every file together.

**This step: zero errors** across all twelve files after the full
cookie-auth rewrite (all `localStorage`/token call sites removed,
`credentials: "include"` and the refresh interceptor added, two new
"Log out" buttons wired in) — confirming the refactor didn't leave any
stale references or type mismatches behind, not just that the new code
in isolation compiles.

This confirms type-correctness and internal consistency — imports
resolve, the API response shapes line up with how they're used — but
**not** runtime behavior (cookies actually round-tripping correctly in
a real browser, the silent-refresh interceptor actually firing at the
right moment). The backend side of the cookie flow *was* verified live
against a real browser-equivalent client (curl with a real cookie jar —
see the backend README's "Verification" section for the full list,
including the refresh-token-reuse attack simulated for real); the
frontend's `fetch`-based equivalent of that same flow has not been
exercised in an actual browser, since this sandbox doesn't have one.
Worth a manual smoke test once this is in your project — particularly
logging in, waiting past 15 minutes (or temporarily lowering
`ACCESS_TOKEN_TTL` for a faster test, as the backend verification did),
and confirming the app keeps working without an unexpected logout.

## Known gaps / good next steps

- The pricing catalog is fetched twice on the customize page (once
  inside `Visualizer`, once in the page itself) — each stays
  self-contained by design; lift `fetchPricingCatalog()` up and pass
  `catalog` as a prop if the duplicate request bothers you.
- Shipping address is a single free-text field wrapped as
  `{"raw": "..."}` JSON — a real store will eventually want structured
  `line1`/`city`/`postal_code`/`country` fields (a matching backend
  schema change too, since it's currently just a JSONB blob).
- `app/customize/page.tsx` still inlines its own copies of
  `inputClass`/`labelClass` rather than importing them from
  `lib/styles.ts` (which postdates it) — cosmetic, easy follow-up.
- No "session about to expire" warning or idle-timeout UX — the silent
  refresh means sessions just keep working invisibly as long as the
  refresh token is valid (7 days by default), which is probably what
  you want, but worth knowing it's there.
