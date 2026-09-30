"use client";

// components/ProductImage.tsx
//
// Renders a product photo when available, falling back to a plain
// gem-outline placeholder when `src` is empty OR the image fails to
// load. That second case matters right now specifically:
// seed/products.json on the backend references paths like
// /images/products/ring-001.jpg that don't exist until real photos are
// actually placed in your project's public/images/products/ directory
// (see the README) — so every card shows this placeholder, not a
// broken-image icon, until then.

import { useState } from "react";

interface ProductImageProps {
  src?: string;
  alt: string;
  className?: string;
}

export default function ProductImage({ src, alt, className }: ProductImageProps) {
  const [failed, setFailed] = useState(false);

  if (!src || failed) {
    return (
      <div
        role="img"
        aria-label={alt}
        className={`flex items-center justify-center bg-gradient-to-br from-[#1F1B17] to-[#2A231D] ${className ?? ""}`}
      >
        <svg width="36" height="36" viewBox="0 0 24 24" fill="none" className="text-[#6E6459]">
          <path d="M12 2 L20 7 V17 L12 22 L4 17 V7 Z" stroke="currentColor" strokeWidth="1.2" />
          <circle cx="12" cy="12" r="3.2" stroke="currentColor" strokeWidth="1.2" />
        </svg>
      </div>
    );
  }

  return (
    // eslint-disable-next-line @next/next/no-img-element -- a plain <img>
    // keeps this component portable across projects regardless of
    // next/image configuration.
    <img src={src} alt={alt} className={className} onError={() => setFailed(true)} />
  );
}
