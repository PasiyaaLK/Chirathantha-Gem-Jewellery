"use client";

// app/products/page.tsx
//
// Ready-made product catalogue (Section 5.1: "Product catalogue with
// filters for category and gender"). Uses GET /api/v1/products, which
// already existed on the backend — this page just consumes it for the
// first time.
//
// There's no purchase flow wired up for ready-made items yet — only
// custom orders (POST /api/v1/orders/custom) actually work end to end.
// The proposal calls for a direct "Buy Now" path for ready-made pieces
// (Section 5.1), but that's a real, separate feature (cart/checkout for
// standard orders) that hasn't been built. Rather than ship a button
// that looks functional but isn't, each card links to /customize
// instead — a genuine, working path, just not "buy this exact piece."

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";

import NavBar from "@/components/NavBar";
import ProductImage from "@/components/ProductImage";
import { apiFetch, ApiError } from "@/lib/api";
import { formatCurrency, titleCase } from "@/lib/format";
import { inputClass, labelClass, pageShellClass } from "@/lib/styles";
import type { Gender, JewelryCategory, Product } from "@/lib/types";

const CATEGORY_OPTIONS: { value: JewelryCategory | ""; label: string }[] = [
  { value: "", label: "All categories" },
  { value: "ring", label: "Rings" },
  { value: "bracelet", label: "Bracelets" },
  { value: "necklace", label: "Necklaces" },
];

const GENDER_OPTIONS: { value: Gender | ""; label: string }[] = [
  { value: "", label: "All" },
  { value: "women", label: "Women's" },
  { value: "men", label: "Men's" },
  { value: "unisex", label: "Unisex" },
];

export default function ProductsPage() {
  const [category, setCategory] = useState<JewelryCategory | "">("");
  const [gender, setGender] = useState<Gender | "">("");
  const [products, setProducts] = useState<Product[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const queryString = useMemo(() => {
    const params = new URLSearchParams();
    if (category) params.set("category", category);
    if (gender) params.set("gender", gender);
    const qs = params.toString();
    return qs ? `?${qs}` : "";
  }, [category, gender]);

  useEffect(() => {
    setError(null);
    apiFetch<Product[]>(`/api/v1/products${queryString}`)
      .then(setProducts)
      .catch((err) =>
        setError(err instanceof ApiError ? err.message : "Failed to load the catalogue."),
      );
  }, [queryString]);

  return (
    <div className={pageShellClass}>
      <NavBar />

      <div className="mx-auto max-w-6xl px-4 py-12">
        <h1 className="font-serif text-3xl">Catalogue</h1>
        <p className="mt-2 text-[#B8AD9E]">
          Ready-made pieces, in stock now. Want something different?{" "}
          <Link href="/customize" className="text-[#C9A46A] hover:underline">
            Design your own
          </Link>
          .
        </p>

        <div className="mt-6 flex flex-wrap gap-4">
          <div>
            <label className={labelClass}>Category</label>
            <select
              className={inputClass}
              value={category}
              onChange={(e) => setCategory(e.target.value as JewelryCategory | "")}
            >
              {CATEGORY_OPTIONS.map((c) => (
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
              value={gender}
              onChange={(e) => setGender(e.target.value as Gender | "")}
            >
              {GENDER_OPTIONS.map((g) => (
                <option key={g.value} value={g.value}>
                  {g.label}
                </option>
              ))}
            </select>
          </div>
        </div>

        {error && <p className="mt-6 text-sm text-[#D96C55]">{error}</p>}

        {products && products.length === 0 && !error && (
          <p className="mt-10 text-[#B8AD9E]">No pieces match those filters right now.</p>
        )}

        {products && products.length > 0 && (
          <div className="mt-8 grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
            {products.map((p) => (
              <div
                key={p.id}
                className="overflow-hidden rounded-lg border border-[#332C25] bg-[#1F1B17]"
              >
                <ProductImage
                  src={p.image_url}
                  alt={p.name}
                  className="aspect-square w-full object-cover"
                />
                <div className="p-4">
                  <p className="font-medium">{p.name}</p>
                  <p className="mt-1 text-sm text-[#B8AD9E]">
                    {[p.metal_type, p.gemstone_type]
                      .filter((v): v is string => Boolean(v))
                      .map(titleCase)
                      .join(", ")}
                  </p>
                  <div className="mt-3 flex items-center justify-between gap-2">
                    <span className="font-medium text-[#C9A46A]">
                      {formatCurrency(p.base_price)}
                    </span>
                    <Link
                      href="/customize"
                      className="text-sm text-[#B8AD9E] hover:text-[#F3EDE2] hover:underline"
                    >
                      Customize a similar piece
                    </Link>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
