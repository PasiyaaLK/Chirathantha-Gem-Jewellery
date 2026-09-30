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
components/NavBar.tsx
components/ProductImage.tsx
app/page.tsx
app/products/page.tsx
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
   `FRONTEND_ORIGIN` CORS default of `http://localhost:3000`.

## Product photos: how to actually add them

`components/ProductImage.tsx` renders whatever `src` it's given, and
falls back to a plain gem-outline placeholder — not a broken-image icon
— when `src` is empty or the file 404s. Right now every seeded product
(`seed/products.json` on the backend) points at a path like
`/images/products/ring-001.jpg` that doesn't exist yet, so every card
shows the placeholder until you add real files.

**To add real photos**: save the image files you have rights to use,
then place them in this project's `public/images/products/` directory
under the exact filenames `seed/products.json` references (or edit
those paths to match whatever you save). For the landing page's hero
image, save one to `public/images/hero.jpg` and pass
`src="/images/hero.jpg"` to the `ProductImage` in `app/page.tsx` (it's
currently called with no `src`, showing the placeholder).

This deliberately does **not** hotlink any third-party URL (an
Instagram CDN link, a Google Images result, etc.) — those are
signed/expiring and unreliable in production regardless of rights, and
using someone else's product photography without clear permission is a
separate concern this sidesteps entirely by having you supply files you
have the rights to use.

## Auth model: HttpOnly cookies, not localStorage

Login/signup/refresh set the access/refresh token pair as `HttpOnly;
Secure; SameSite=Strict` cookies — see the backend README's "Auth
model" section for the full design. What that means here:

- **No `getToken()`/`setToken()`/`clearToken()`** — an HttpOnly cookie
  is invisible to JS by design. `lib/api.ts` sends `credentials:
  "include"` on every request instead.
- **`lib/useAuth.ts`'s `useCurrentUser()`** always calls `GET /auth/me`
  and treats any failure as "not logged in" — that's the only way to know.
- **Silent access-token refresh**: `apiFetch` catches a 401 on any
  non-auth endpoint, attempts one `POST /api/v1/auth/refresh`, and
  retries once if that succeeds — necessary given a 15-minute access
  token. Concurrent 401s share one in-flight refresh (see
  `refreshInFlight`) rather than racing, since the backend rotates the
  refresh token on every use and a second concurrent refresh would look
  like a replay attack.
- **`logout()`** (in `lib/api.ts`) is the only way to log out anymore —
  wired into `components/NavBar.tsx`.

## What each file does

- **`lib/types.ts`** — TypeScript mirrors of the Go backend's JSON shapes.
- **`lib/pricing.ts`** — fetches `GET /api/v1/pricing/catalog` and
  computes a price estimate from the real numbers.
- **`lib/api.ts`** — cookie-based `fetch` wrapper, silent-refresh
  interceptor, `logout()`, `ApiError` parsing.
- **`lib/styles.ts`** — shared Tailwind class strings and the
  `STATUS_BADGE` map.
- **`lib/format.ts`** — `titleCase`, `formatDate`, `formatCurrency`,
  `formatShippingAddress`.
- **`lib/useAuth.ts`** — `useCurrentUser()`.
- **`components/Visualizer.tsx`** — Three.js 3D customization preview
  (procedural geometry, no real asset pipeline) with a live price badge.
- **`components/NavBar.tsx`** — shared header on every page: Catalogue/
  Customize links always visible; My Orders/Dashboard (role-aware) and
  Log out once `useCurrentUser` resolves a logged-in user; Log in
  otherwise. Nothing in that slot renders while still loading, so a
  logged-in admin never flashes a "Log in" link on refresh.
- **`components/ProductImage.tsx`** — photo-or-placeholder, see above.
- **`app/page.tsx`** — landing page: hero section (headline, two CTAs
  into `/products` and `/customize`, a placeholder-until-you-add-one
  hero image) plus a three-feature summary of how the store works.
- **`app/products/page.tsx`** — the ready-made catalogue, filterable by
  category/gender, consuming `GET /api/v1/products` for the first time
  in this frontend. **No working "Buy Now"** — only custom orders are
  wired up end to end on the backend (see the backend README's
  assumptions list); each card links to `/customize` instead of a
  non-functional purchase button, which was a deliberate call not to
  ship something that looks functional but isn't.
- **`app/customize/page.tsx`** — the full customization form.
- **`app/login/page.tsx`** / **`app/signup/page.tsx`** — auth forms,
  routing by role after login.
- **`app/admin/dashboard/page.tsx`** — role-protected pending-orders
  queue with an inspection drawer and Approve/Decline actions.
- **`app/orders/page.tsx`** — the customer's own order history with
  status badges.

## Backend changes required to support these pages (cumulative)

1. `GET /api/v1/orders` (order history) and full item enrichment
   (`Customization`/`Product` nested in each `OrderItem`, not a bare ID).
2. `shipping_address` on the admin pending-orders response.
3. Cookie-based auth + refresh/logout endpoints — an older backend copy
   would still return a JSON `token` field these pages no longer read,
   and nothing would actually authenticate.
4. **This step**: `ProductRepository.Create` and `cmd/seedproducts` —
   `GET /api/v1/products` existed already, but nothing had ever created
   a product, so it always returned an empty catalogue until now.

## Verification performed

Every version of this frontend has been type-checked for real: a
scratch npm project with the real `next`/`react`/`three` packages and
`@types/*`, a standard `tsconfig.json` with the `@/*` alias, and
`tsc --noEmit --strict` (plus, this step, `noUnusedLocals` /
`noUnusedParameters` — stricter than any previous pass) run across
every file together.

**This actually caught two real mistakes before they shipped**, both
introduced while wiring `NavBar` into the six existing pages:

1. A genuine JSX syntax error in `app/customize/page.tsx` — one of the
   two return blocks there only needed `<NavBar />` inserted between two
   *already-existing* wrapper divs (no new div, so no new closing tag
   needed), but I'd mechanically added a closing tag there anyway,
   copying the pattern from a different block that *did* add a new
   wrapper. `noUnusedLocals` and a strict parse both would have caught
   this eventually; catching it now means it never reaches you.
2. A stale unused variable in `app/signup/page.tsx`, left over from an
   earlier step that removed a `setToken(result.token)` call without
   noticing `result` then had no other use.

Both fixed, then re-verified clean. This confirms type-correctness,
internal consistency, and (via the unused-locals check this time) that
the multi-file NavBar rewiring didn't leave dead code behind — not
runtime behavior in an actual browser, which this sandbox can't do.
Worth a manual smoke test once this is in your project.

## Known gaps / good next steps

- No product detail page — catalogue cards show name/price/materials
  only, no click-through.
- No working ready-made checkout (see `app/products/page.tsx` above).
- The pricing catalog is fetched twice on the customize page (once
  inside `Visualizer`, once in the page) — each stays self-contained by
  design; lift the fetch up and pass `catalog` as a prop if that
  duplicate request bothers you.
- Shipping address is a single free-text field wrapped as
  `{"raw": "..."}` JSON — a real store will eventually want structured
  `line1`/`city`/`postal_code`/`country` fields.
- `app/customize/page.tsx` still inlines its own copies of
  `inputClass`/`labelClass` rather than importing from `lib/styles.ts`
  (which postdates it) — cosmetic, easy follow-up.
- No "session about to expire" warning — the silent refresh just keeps
  working invisibly as long as the refresh token is valid (7 days by
  default).
