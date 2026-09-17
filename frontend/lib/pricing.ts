// lib/pricing.ts
//
// Client-side mirror of internal/service/pricing.go's Calculate logic.
// Instead of hardcoding a second copy of the price numbers — which WILL
// drift the moment someone updates prices on only one side — this
// fetches the live catalog from GET /api/v1/pricing/catalog and computes
// against those exact numbers. "Client-side pricing that matches the
// server's rules" is enforced by construction here, not by two people
// remembering to keep a Go file and a TypeScript file in sync.
//
// The FORMULA itself (base * metalMultiplier + gemstoneFee +
// engravingBaseFee + text.length * engravingPerCharFee) is still
// duplicated — that's inherent to wanting an instant, no-network-round-
// trip estimate as the user types. What's NOT duplicated is the price
// DATA, which is the part that actually changes over time. The server
// (PricingService.Calculate) is still the only source of truth for what
// gets charged; this is a preview, nothing more.

import { apiFetch } from "./api";
import type { JewelryCategory, PricingCatalog } from "./types";

export async function fetchPricingCatalog(): Promise<PricingCatalog> {
  return apiFetch<PricingCatalog>("/api/v1/pricing/catalog");
}

export interface EstimateInput {
  category: JewelryCategory;
  metalType: string;
  /** "none" or empty means no gemstone. */
  gemstoneType?: string;
  engravingText?: string;
}

export interface EstimateResult {
  total: number;
  breakdown: {
    categoryBase: number;
    metalMultiplier: number;
    afterMetal: number;
    gemstoneFee: number;
    engravingFee: number;
  };
  /**
   * False when metal/gemstone don't match a catalog entry — e.g. while
   * the user hasn't picked a metal yet, or a stale catalog is missing a
   * key the server now recognizes. `total` is not meaningful when this
   * is false; show a placeholder instead of the number.
   */
  valid: boolean;
}

function round2(n: number): number {
  return Math.round(n * 100) / 100;
}

export function estimatePrice(
  input: EstimateInput,
  catalog: PricingCatalog,
): EstimateResult {
  const categoryBase = catalog.category_base_prices[input.category] ?? 0;

  const metalKey = input.metalType.trim().toLowerCase();
  const metalMultiplier = catalog.metal_multipliers[metalKey];
  const validMetal = metalMultiplier !== undefined;

  const gemKey = input.gemstoneType?.trim().toLowerCase() || "none";
  const gemstoneFee = gemKey === "none" ? 0 : catalog.gemstone_fees[gemKey];
  const validGem = gemKey === "none" || gemstoneFee !== undefined;

  const afterMetal = round2(categoryBase * (metalMultiplier ?? 0));

  const text = input.engravingText?.trim() ?? "";
  const engravingFee =
    text.length > 0
      ? round2(
          catalog.engraving_base_fee + text.length * catalog.engraving_per_char_fee,
        )
      : 0;

  const total = round2(afterMetal + (gemstoneFee ?? 0) + engravingFee);

  return {
    total,
    breakdown: {
      categoryBase,
      metalMultiplier: metalMultiplier ?? 0,
      afterMetal,
      gemstoneFee: gemstoneFee ?? 0,
      engravingFee,
    },
    valid: validMetal && validGem,
  };
}

/**
 * Wraps a plain address string as valid JSON for the
 * `shipping_address` field — orders.shipping_address is a JSONB column
 * server-side, and a bare string like "123 Main St" is NOT valid JSON
 * (it needs to be a quoted JSON string, an object, etc.) — the Go
 * service now 400s on anything that isn't, rather than the request
 * failing deep in a DB transaction. Returns undefined for blank input so
 * the field is omitted entirely rather than sent as `{"raw": ""}`.
 */
export function buildShippingAddressJSON(raw: string): string | undefined {
  const trimmed = raw.trim();
  if (!trimmed) return undefined;
  return JSON.stringify({ raw: trimmed });
}
