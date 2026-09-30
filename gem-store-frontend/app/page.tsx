// app/page.tsx
//
// Landing page. No client hooks of its own — NavBar (a client
// component) is imported and rendered like any other component, which
// Next.js handles across the server/client boundary automatically.

import Link from "next/link";

import NavBar from "@/components/NavBar";
import ProductImage from "@/components/ProductImage";
import { buttonPrimaryClass, buttonSecondaryClass, pageShellClass } from "@/lib/styles";

export default function HomePage() {
  return (
    <div className={pageShellClass}>
      <NavBar />

      <section className="mx-auto grid max-w-6xl items-center gap-10 px-4 py-16 md:grid-cols-2 md:py-24">
        <div>
          <h1 className="font-serif text-4xl leading-tight md:text-5xl">
            Fine jewelry, made to your exact design
          </h1>
          <p className="mt-4 max-w-md text-[#B8AD9E]">
            Browse ready-made pieces, or design something entirely your own —
            choose the metal, the stone, the engraving — and watch it come
            together in a live preview before you order.
          </p>
          <div className="mt-8 flex flex-wrap gap-4">
            <Link href="/products" className={`${buttonPrimaryClass} inline-block`}>
              Browse the catalogue
            </Link>
            <Link href="/customize" className={`${buttonSecondaryClass} inline-block`}>
              Design something custom
            </Link>
          </div>
        </div>

        {/* No hero photo wired in yet — see the README for how to add
            one once you have real photography: drop it in
            public/images/hero.jpg and pass src="/images/hero.jpg" here. */}
        <ProductImage
          alt="Featured piece"
          className="aspect-square w-full rounded-lg border border-[#332C25]"
        />
      </section>

      <section className="border-t border-[#332C25] bg-[#1F1B17]/40">
        <div className="mx-auto grid max-w-6xl gap-8 px-4 py-16 md:grid-cols-3">
          <Feature
            title="Browse ready-made"
            description="Rings, bracelets, and necklaces for men and women, ready to order directly."
          />
          <Feature
            title="Design something custom"
            description="Pick your metal, gemstone, size, and engraving, and see a live 3D preview as you go."
          />
          <Feature
            title="Reviewed by hand"
            description="Every custom order is checked for feasibility before it's confirmed — you'll hear back by email or SMS."
          />
        </div>
      </section>
    </div>
  );
}

function Feature({ title, description }: { title: string; description: string }) {
  return (
    <div>
      <h3 className="font-serif text-xl">{title}</h3>
      <p className="mt-2 text-sm text-[#B8AD9E]">{description}</p>
    </div>
  );
}
