# Gem & Jewelry Store — Frontend pieces (customization workspace)

Five files, meant to be dropped into an existing Next.js 14+ (App
Router) + TypeScript project at these exact relative paths:

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

**One correction from the request:** the Go backend's real signup route
is `POST /api/v1/auth/signup` (built in an earlier step), not
`/api/v1/auth/register` — the login/signup pages below call the route
that actually exists.

## Prerequisites / assumptions

1. **Next.js App Router**, not Pages Router — matches the
   `app/customize/page.tsx` path you asked for.
2. **The `@/*` import alias** resolving to your project root (the
   default in any project scaffolded with `create-next-app` +
   TypeScript). If yours differs, adjust the `@/lib/...` and
   `@/components/...` imports.
3. **Tailwind CSS** is assumed for styling — all classes use arbitrary
   hex values (`bg-[#161310]`) rather than theme tokens, so they work
   regardless of your `tailwind.config` contents. If you're not on
   Tailwind, the logic is unaffected; swap the `className` strings for
   your styling approach.
4. **`three` is a new dependency**: `npm install three` (and
   `npm install -D @types/three` for TypeScript). Verified against
   `three@^0.160.0`'s real type definitions — see below.
5. **`NEXT_PUBLIC_API_BASE_URL`** env var — set it in `.env.local` to
   your Go backend's URL (defaults to `http://localhost:8080`, which
   matches the backend's own default port). The backend's
   `FRONTEND_ORIGIN` CORS setting already defaults to
   `http://localhost:3000`, the standard Next.js dev port, so local dev
   should work with zero config on either side.
6. **JWT storage: localStorage**, under the key `gemstore_token`. This
   is the simplest thing that works before a real login page exists in
   this frontend — but it's readable by any script on the page, which
   makes it vulnerable to token theft via XSS. `lib/api.ts` isolates
   this behind `getToken()`/`setToken()`/`clearToken()`, so swapping to
   an httpOnly cookie set by a Next.js Route Handler (the recommended
   production pattern) later only means changing those three functions
   — nothing that calls them needs to change. For now, after your
   login/signup call succeeds, call `setToken(response.token)`
   yourself (not wired up here — no login page was in scope this step).

## What each file does

- **`lib/types.ts`** — TypeScript mirrors of the Go backend's JSON
  shapes (`internal/models`). Hand-maintained, except pricing data
  specifically (see next).
- **`lib/pricing.ts`** — fetches `GET /api/v1/pricing/catalog` (new
  this step) and computes a price estimate from the *real* numbers,
  rather than a hardcoded second copy that could silently drift from
  the Go source. The formula itself is still duplicated — that's
  inherent to wanting an instant, no-round-trip estimate as the user
  types — but the data isn't.
- **`lib/api.ts`** — thin `fetch` wrapper: base URL, JSON headers, auth
  header injection, and parsing the Go backend's `{"error": "..."}`
  shape into a typed `ApiError`.
- **`components/Visualizer.tsx`** — a Three.js 3D preview (procedural
  geometry — no jewelry asset pipeline exists, so a ring/bracelet/
  necklace band is a torus, not a modeled ring) with material driven by
  the selected metal and a small mesh for the gemstone, plus a live
  price badge. Engraving is shown as a caption below the preview rather
  than etched into the 3D surface — true circumferential text on a
  curved band needs `TextGeometry` projected onto a curve, which is a
  reasonable follow-up if 3D fidelity matters more than the
  pricing/order integration for your purposes right now.
- **`app/customize/page.tsx`** — the full form: category/gender/metal/
  gemstone/size/engraving/shipping address/quantity, wired to
  `Visualizer` for the live preview and to
  `POST /api/v1/orders/custom` for submission. Handles the "not logged
  in," validation-error (400 from the pricing engine or field
  validation), and success states.
- **`lib/styles.ts`** — shared Tailwind class strings and the
  `STATUS_BADGE` map (one color/label per `OrderStatus`) so the new
  pages below share one visual language instead of each hand-rolling
  input/button styling. (`customize/page.tsx` predates this file and
  still inlines its own copies — not refactored here since that wasn't
  part of this step's ask, but pointing it at this file is a trivial
  follow-up.)
- **`lib/format.ts`** — `titleCase`, `formatDate`, `formatCurrency`, and
  `formatShippingAddress` (unwraps the `{"raw": "..."}` JSON the
  customize page sends back into a plain string for display).
- **`lib/useAuth.ts`** — `useCurrentUser()`, a hook that calls
  `GET /api/v1/auth/me` with whatever token is stored and exposes
  `{ user, loading, error }`. Both new protected pages depend on
  `loading` specifically: redirecting (or showing protected content) is
  only safe once the check has actually settled — collapsing "still
  checking" and "definitely not logged in" into one state is how a
  protected page ends up flashing its content before redirecting.
- **`app/login/page.tsx`** / **`app/signup/page.tsx`** — forms calling
  the real backend routes, storing the returned token via
  `lib/api.ts`'s `setToken`, and routing by role after login (admin →
  `/admin/dashboard`, customer → `/orders`).
- **`app/admin/dashboard/page.tsx`** — protected by `useCurrentUser`
  (redirects to `/login` once it's *certain* the caller isn't an admin,
  never before the check settles); lists
  `GET /api/v1/admin/orders/pending`; clicking an order opens a drawer
  with full customization + shipping detail and Approve/Decline actions
  calling the real endpoints, refetching the list on success.
- **`app/orders/page.tsx`** — the customer's own order history from the
  new `GET /api/v1/orders` backend endpoint (added this step — see the
  backend README's "What's built" section), with a status badge per
  order from the shared `STATUS_BADGE` map.

## Backend changes required to support these pages

Building these pages surfaced two real gaps in the backend, both fixed
(see the backend's own README for detail):

1. **`GET /api/v1/orders` didn't exist.** There was only
   `GET /api/v1/orders/{id}` (single order by ID) — nothing to back an
   order *history* list. Added, scoped to the caller at the SQL level.
2. **Order items only carried a bare `customization_id`/`product_id`
   UUID**, not the actual customization/product detail — the order
   history page couldn't have rendered anything meaningful ("Ring — Rose
   Gold, Sapphire") without it. Every `OrderItem` the API returns now
   carries the full nested object.
3. **`shipping_address` was missing from the admin pending-orders
   response** — present on `Order` but not on `PendingCustomOrder`,
   which the admin drawer explicitly needs. Added.

If you're pointing this frontend at an older copy of the backend that
predates these three fixes, the order-tracking and admin-drawer pages
won't have the data they expect.

## Verification performed

Rather than eyeballing the TypeScript, this was actually type-checked:
a scratch npm project was set up with the **real** `next`, `react`,
and `three` packages (plus their real `@types/*` definitions) at
realistic recent versions, a standard Next.js `tsconfig.json` (with the
`@/*` path alias configured exactly as a real project would have it),
and `tsc --noEmit --strict` run against all twelve files together (the
original five plus the new shared modules and pages from this step).

**Result: zero errors**, in strict mode, against the real type
definitions for React, Three.js, and Node — not a mocked or simplified
type surface. This confirms the code is type-correct and internally
consistent (imports resolve, the `PricingCatalog`/`CustomOrderResult`
shapes line up with how they're used, the Three.js API calls match
real signatures), but it does **not** confirm runtime rendering
behavior (a WebGL canvas actually painting correctly, form
interactions behaving as expected in a browser) — that needs your own
dev server and a browser, which this sandbox doesn't have. Worth
manually smoke-testing the page once it's in your project, particularly
the 3D preview.

## Known gaps / good next steps

- No login page yet — `getToken()` returns `null` until something calls
  `setToken()`. The customize page correctly shows "please log in"
  rather than silently submitting an unauthenticated request, but you
  need a login page calling `POST /api/v1/auth/login` and then
  `setToken(response.token)` before this is usable end-to-end.
- The pricing catalog is fetched twice on this page (once inside
  `Visualizer`, once in the page itself) — each stays self-contained by
  design (see the comment in `Visualizer.tsx`), but if that duplicate
  request bothers you, lift the `fetchPricingCatalog()` call up and
  pass `catalog` into `Visualizer` as a prop.
- Shipping address is a single free-text field wrapped as
  `{"raw": "..."}` JSON — fine for now, but a real store will want
  structured `line1`/`city`/`postal_code`/`country` fields, which would
  also need a matching schema change on the Go side (currently just a
  JSONB blob).
