# Gem & Jewelry Store — Frontend pieces (customization workspace)

Five files, meant to be dropped into an existing Next.js 14+ (App
Router) + TypeScript project at these exact relative paths:

```
lib/types.ts
lib/api.ts
lib/pricing.ts
components/Visualizer.tsx
app/customize/page.tsx
```

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

## Verification performed

Rather than eyeballing the TypeScript, this was actually type-checked:
a scratch npm project was set up with the **real** `next`, `react`,
and `three` packages (plus their real `@types/*` definitions) at
realistic recent versions, a standard Next.js `tsconfig.json` (with the
`@/*` path alias configured exactly as a real project would have it),
and `tsc --noEmit --strict` run against all five files together.

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
