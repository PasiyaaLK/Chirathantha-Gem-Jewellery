"use client";

// app/customize/page.tsx
//
// The customer-facing customization workspace (Section 5.1 of the
// project proposal): pick category/metal/gemstone/engraving, watch the
// 3D preview and price update live, submit to
// POST /api/v1/orders/custom. On success the order sits in
// "pending_review" until the store owner acts on it (Step 2's admin
// approval workflow) and the customer is notified by email/SMS
// (Step 2's notifier) once a decision is made.
//
// STYLING NOTE: uses Tailwind utility classes with arbitrary hex values
// rather than theme tokens, so this file works standalone regardless of
// your tailwind.config — swap in real theme tokens (colors.ink,
// colors.garnet, etc.) if you'd rather reference a shared palette.
// Heading uses `font-serif` (a zero-config system serif) for contrast
// against the sans-serif UI text; swapping in a loaded display face
// (e.g. Fraunces via next/font/google in your root layout) would carry
// the "fine jewelry atelier" feel further than the system serif can.

import { useEffect, useMemo, useState, type FormEvent } from "react";

import Visualizer, { type VisualizerSelection } from "@/components/Visualizer";
import { apiFetch, ApiError, getToken } from "@/lib/api";
import { buildShippingAddressJSON, estimatePrice, fetchPricingCatalog } from "@/lib/pricing";
import type {
  CustomOrderRequest,
  CustomOrderResult,
  Gender,
  JewelryCategory,
  PricingCatalog,
} from "@/lib/types";

const CATEGORIES: { value: JewelryCategory; label: string }[] = [
  { value: "ring", label: "Ring" },
  { value: "bracelet", label: "Bracelet" },
  { value: "necklace", label: "Necklace" },
];

const GENDERS: { value: Gender; label: string }[] = [
  { value: "women", label: "Women's" },
  { value: "men", label: "Men's" },
  { value: "unisex", label: "Unisex" },
];

// Mirrors validateCustomOrderRequest's engraving-length check in
// internal/service/order_service.go — client-side enforcement is just
// UX; the server's is the one that actually matters.
const MAX_ENGRAVING_CHARS = 100;

interface FormState {
  category: JewelryCategory;
  gender: Gender;
  metalType: string;
  /** "none" or a catalog key from PricingCatalog.gemstone_fees. */
  gemstoneType: string;
  size: string;
  engravingText: string;
  shippingAddress: string;
  quantity: number;
}

const initialForm: FormState = {
  category: "ring",
  gender: "women",
  metalType: "",
  gemstoneType: "none",
  size: "",
  engravingText: "",
  shippingAddress: "",
  quantity: 1,
};

type SubmitState =
  | { status: "idle" }
  | { status: "submitting" }
  | { status: "success"; result: CustomOrderResult }
  | { status: "error"; message: string };

function titleCase(s: string): string {
  return s.replace(/\b\w/g, (c) => c.toUpperCase());
}

const inputClass =
  "mt-1 w-full rounded-md border border-[#332C25] bg-[#1F1B17] px-3 py-2 text-[#F3EDE2] " +
  "focus:border-[#C9A46A] focus:outline-none focus:ring-1 focus:ring-[#C9A46A] disabled:opacity-50";
const labelClass = "block text-sm text-[#B8AD9E]";

export default function CustomizePage() {
  const [form, setForm] = useState<FormState>(initialForm);
  const [catalog, setCatalog] = useState<PricingCatalog | null>(null);
  const [catalogError, setCatalogError] = useState<string | null>(null);
  const [submitState, setSubmitState] = useState<SubmitState>({ status: "idle" });

  useEffect(() => {
    fetchPricingCatalog()
      .then((c) => {
        setCatalog(c);
        // Default to the catalog's first metal once it loads, rather
        // than hardcoding a guess that might not match what the server
        // actually recognizes.
        setForm((f) =>
          f.metalType ? f : { ...f, metalType: Object.keys(c.metal_multipliers)[0] ?? "" },
        );
      })
      .catch((err) =>
        setCatalogError(err instanceof Error ? err.message : "Failed to load pricing catalog"),
      );
  }, []);

  const estimate = useMemo(() => {
    if (!catalog) return null;
    return estimatePrice(
      {
        category: form.category,
        metalType: form.metalType,
        gemstoneType: form.gemstoneType,
        engravingText: form.engravingText,
      },
      catalog,
    );
  }, [catalog, form.category, form.metalType, form.gemstoneType, form.engravingText]);

  const visualizerSelection: VisualizerSelection = {
    category: form.category,
    metalType: form.metalType,
    gemstoneType: form.gemstoneType,
    engravingText: form.engravingText,
  };

  function updateField<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((f) => ({ ...f, [key]: value }));
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();

    if (!getToken()) {
      setSubmitState({ status: "error", message: "Please log in before submitting a custom order." });
      return;
    }

    setSubmitState({ status: "submitting" });

    const payload: CustomOrderRequest = {
      category: form.category,
      gender: form.gender,
      metal_type: form.metalType,
      gemstone_type: form.gemstoneType === "none" ? undefined : form.gemstoneType,
      size: form.size.trim() || undefined,
      engraving_text: form.engravingText.trim() || undefined,
      estimated_price: estimate?.total ?? 0,
      quantity: form.quantity,
      shipping_address: buildShippingAddressJSON(form.shippingAddress),
    };

    try {
      const result = await apiFetch<CustomOrderResult>("/api/v1/orders/custom", {
        method: "POST",
        auth: true,
        body: payload,
      });
      setSubmitState({ status: "success", result });
    } catch (err) {
      // 400s here are almost always the pricing engine or a validation
      // rule rejecting something specific (an unrecognized metal, bad
      // shipping_address JSON) — surface the server's actual reason
      // rather than a generic "something went wrong."
      if (err instanceof ApiError) {
        setSubmitState({ status: "error", message: err.message });
      } else {
        setSubmitState({ status: "error", message: "Something went wrong submitting your order." });
      }
    }
  }

  if (submitState.status === "success") {
    const { order, pricing } = submitState.result;
    return (
      <div className="mx-auto min-h-screen max-w-lg bg-[#161310] px-4 py-16 text-center text-[#F3EDE2]">
        <h1 className="font-serif text-3xl">Your design is in review</h1>
        <p className="mt-3 text-[#B8AD9E]">
          We&apos;ll email you as soon as it&apos;s confirmed or if we need to
          adjust anything.
        </p>

        <dl className="mx-auto mt-8 max-w-xs space-y-2 border-t border-[#332C25] pt-6 text-sm">
          <div className="flex justify-between">
            <dt className="text-[#B8AD9E]">Order reference</dt>
            <dd className="font-mono">{order.id.slice(0, 8)}</dd>
          </div>
          <div className="flex justify-between">
            <dt className="text-[#B8AD9E]">Status</dt>
            <dd className="capitalize">{order.status.replace("_", " ")}</dd>
          </div>
          <div className="flex justify-between text-base font-medium">
            <dt>Total</dt>
            <dd className="text-[#C9A46A]">${pricing.total.toFixed(2)}</dd>
          </div>
        </dl>

        <button
          type="button"
          onClick={() => {
            setForm(initialForm);
            setSubmitState({ status: "idle" });
          }}
          className="mt-10 rounded-md border border-[#C9A46A] px-5 py-2 text-sm text-[#C9A46A] transition-colors hover:bg-[#C9A46A] hover:text-[#161310]"
        >
          Design another piece
        </button>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-[#161310] text-[#F3EDE2]">
      <div className="mx-auto grid max-w-4xl gap-10 px-4 py-12 md:grid-cols-2 md:gap-12">
        <div>
          <h1 className="font-serif text-3xl">Design your piece</h1>
          <p className="mt-2 max-w-sm text-[#B8AD9E]">
            Choose your materials — the preview and price update as you go.
            Nothing is charged until we confirm your design is feasible to
            make.
          </p>

          <Visualizer selection={visualizerSelection} className="mt-8" />
        </div>

        <form onSubmit={handleSubmit} className="space-y-5">
          <div>
            <label className={labelClass}>Category</label>
            <select
              className={inputClass}
              value={form.category}
              onChange={(e) => updateField("category", e.target.value as JewelryCategory)}
            >
              {CATEGORIES.map((c) => (
                <option key={c.value} value={c.value}>
                  {c.label}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className={labelClass}>For</label>
            <select
              className={inputClass}
              value={form.gender}
              onChange={(e) => updateField("gender", e.target.value as Gender)}
            >
              {GENDERS.map((g) => (
                <option key={g.value} value={g.value}>
                  {g.label}
                </option>
              ))}
            </select>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className={labelClass}>Metal</label>
              <select
                className={inputClass}
                value={form.metalType}
                disabled={!catalog}
                onChange={(e) => updateField("metalType", e.target.value)}
              >
                {catalog &&
                  Object.keys(catalog.metal_multipliers).map((key) => (
                    <option key={key} value={key}>
                      {titleCase(key)}
                    </option>
                  ))}
              </select>
            </div>

            <div>
              <label className={labelClass}>Gemstone</label>
              <select
                className={inputClass}
                value={form.gemstoneType}
                disabled={!catalog}
                onChange={(e) => updateField("gemstoneType", e.target.value)}
              >
                <option value="none">None</option>
                {catalog &&
                  Object.keys(catalog.gemstone_fees).map((key) => (
                    <option key={key} value={key}>
                      {titleCase(key)}
                    </option>
                  ))}
              </select>
            </div>
          </div>

          <div>
            <label className={labelClass}>Size</label>
            <input
              className={inputClass}
              value={form.size}
              onChange={(e) => updateField("size", e.target.value)}
              placeholder={form.category === "ring" ? "e.g. US 6" : "e.g. 18cm"}
            />
          </div>

          <div>
            <label className={labelClass}>
              Engraving{" "}
              <span className="text-[#6E6459]">
                ({form.engravingText.length}/{MAX_ENGRAVING_CHARS})
              </span>
            </label>
            <input
              className={inputClass}
              value={form.engravingText}
              maxLength={MAX_ENGRAVING_CHARS}
              onChange={(e) => updateField("engravingText", e.target.value)}
              placeholder="Optional"
            />
          </div>

          <div>
            <label className={labelClass}>Shipping address</label>
            <textarea
              className={inputClass}
              rows={2}
              value={form.shippingAddress}
              onChange={(e) => updateField("shippingAddress", e.target.value)}
              placeholder="Optional — can be added later"
            />
          </div>

          <div>
            <label className={labelClass}>Quantity</label>
            <input
              type="number"
              min={1}
              className={`${inputClass} w-24`}
              value={form.quantity}
              onChange={(e) => updateField("quantity", Math.max(1, Number(e.target.value) || 1))}
            />
          </div>

          <div className="flex items-center justify-between border-t border-[#332C25] pt-5">
            <span className="text-sm text-[#B8AD9E]">Estimated total</span>
            <span className="text-xl font-medium text-[#C9A46A]">
              {estimate?.valid ? `$${estimate.total.toFixed(2)}` : "—"}
            </span>
          </div>

          {catalogError && (
            <p className="text-sm text-[#B8AD9E]">
              Couldn&apos;t load live pricing — you can still submit; the
              server always calculates the final price.
            </p>
          )}
          {submitState.status === "error" && (
            <p className="text-sm text-[#D96C55]">{submitState.message}</p>
          )}

          <button
            type="submit"
            disabled={submitState.status === "submitting" || !catalog}
            className="w-full rounded-md bg-[#8C3B4A] px-4 py-3 font-medium text-[#F3EDE2] transition-opacity hover:opacity-90 disabled:opacity-40"
          >
            {submitState.status === "submitting" ? "Submitting…" : "Submit custom order"}
          </button>
        </form>
      </div>
    </div>
  );
}
