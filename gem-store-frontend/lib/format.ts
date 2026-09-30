// lib/format.ts

export function titleCase(s: string): string {
  return s.replace(/\b\w/g, (c) => c.toUpperCase());
}

/**
 * orders.shipping_address is stored as raw JSONB text — since
 * buildShippingAddressJSON (lib/pricing.ts) wraps whatever the customer
 * typed as `{"raw": "..."}`, what comes back from the API is literally
 * that JSON text (quotes and all), not the plain address string. This
 * unwraps it for display, falling back to showing the raw stored value
 * if it's missing or doesn't match the expected shape (e.g. a future
 * structured address object) rather than silently hiding it.
 */
export function formatShippingAddress(raw: string | undefined): string {
  if (!raw) return "Not provided";
  try {
    const parsed = JSON.parse(raw) as { raw?: unknown };
    if (typeof parsed.raw === "string" && parsed.raw.trim()) return parsed.raw;
  } catch {
    // Not JSON, or not the expected shape — fall through.
  }
  return raw;
}

export function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

export function formatCurrency(amount: number): string {
  return `$${amount.toFixed(2)}`;
}
