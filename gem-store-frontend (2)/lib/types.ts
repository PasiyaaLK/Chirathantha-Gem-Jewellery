// lib/types.ts
//
// Mirrors the JSON shapes returned by the Go backend (internal/models).
// Kept in sync by hand — the one place this matters for money
// (the pricing catalog) fetches its numbers live instead of duplicating
// them; see lib/pricing.ts.

export type JewelryCategory = "ring" | "bracelet" | "necklace";
export type Gender = "men" | "women" | "unisex";
export type OrderStatus =
  | "cart"
  | "pending_review"
  | "confirmed"
  | "declined"
  | "in_progress"
  | "shipped"
  | "delivered"
  | "cancelled";

export interface User {
  id: string;
  email: string;
  full_name: string;
  phone?: string;
  role: "customer" | "admin";
  created_at: string;
  updated_at: string;
}

export interface Product {
  id: string;
  sku: string;
  name: string;
  description?: string;
  category: JewelryCategory;
  gender: Gender;
  metal_type?: string;
  gemstone_type?: string;
  base_price: number;
  stock_quantity: number;
  image_url?: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface Customization {
  id: string;
  user_id: string;
  category: JewelryCategory;
  gender: Gender;
  metal_type: string;
  gemstone_type?: string;
  size?: string;
  engraving_text?: string;
  preview_image_url?: string;
  estimated_price?: number;
  created_at: string;
  updated_at: string;
}

export interface OrderItem {
  id: string;
  order_id: string;
  product_id?: string;
  customization_id?: string;
  quantity: number;
  unit_price: number;
  created_at: string;
  // Populated by the backend so a client can render "Ring — Rose Gold,
  // Sapphire" without a second round trip. Exactly one of these is ever
  // present per item.
  customization?: Customization;
  product?: Product;
}

export interface Order {
  id: string;
  user_id: string;
  order_type: "standard" | "custom";
  status: OrderStatus;
  // Raw JSONB text as stored — e.g. {"raw": "12 Lake Drive, Kandy"}.
  // See formatShippingAddress in lib/format.ts to render it.
  shipping_address?: string;
  total_amount: number;
  created_at: string;
  updated_at: string;
  items?: OrderItem[];
}

export interface PricingBreakdown {
  category_base: number;
  metal_multiplier: number;
  after_metal: number;
  gemstone_fee: number;
  engraving_fee: number;
  total: number;
}

export interface PricingCatalog {
  category_base_prices: Record<string, number>;
  metal_multipliers: Record<string, number>;
  gemstone_fees: Record<string, number>;
  engraving_base_fee: number;
  engraving_per_char_fee: number;
}

export interface CustomOrderResult {
  order: Order;
  pricing: PricingBreakdown;
}

// PendingCustomOrder is what GET /api/v1/admin/orders/pending returns —
// a flattened, read-optimized view (order + customer contact +
// customization + line quantity/price in one row), not the same shape
// as Order. See internal/models.PendingCustomOrder on the Go side.
export interface PendingCustomOrder {
  order_id: string;
  user_id: string;
  status: OrderStatus;
  total_amount: number;
  shipping_address?: string;
  created_at: string;
  customer_name: string;
  customer_email: string;
  customer_phone?: string;
  quantity: number;
  unit_price: number;
  customization: Customization;
}

// Payload for POST /api/v1/admin/orders/{id}/approve and .../decline.
export interface DecisionRequest {
  notes?: string;
}

// AuthResponse is what signup/login/refresh return. There's no token
// field anymore — the access/refresh tokens travel exclusively as
// HttpOnly cookies the browser manages automatically; see lib/api.ts's
// file-level comment.
export interface AuthResponse {
  user: User;
}

export interface CustomOrderRequest {
  category: JewelryCategory;
  gender: Gender;
  metal_type: string;
  gemstone_type?: string;
  size?: string;
  engraving_text?: string;
  // Sent for transparency/telemetry only. The server (PricingService,
  // see internal/service/pricing.go) recomputes and overrides this from
  // the same catalog every time — never trust it client-side either.
  estimated_price: number;
  quantity: number;
  // Must be a JSON string if present — orders.shipping_address is a
  // JSONB column server-side (validateCustomOrderRequest 400s on
  // anything that isn't valid JSON). See buildShippingAddressJSON below.
  shipping_address?: string;
}
