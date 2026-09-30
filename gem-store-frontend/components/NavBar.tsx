"use client";

// components/NavBar.tsx
//
// Shared header for every page. Shows role-aware links (My Orders vs.
// Dashboard) and a log-out control once useCurrentUser resolves —
// nothing renders in that slot while it's still loading, so a logged-in
// admin never flashes a "Log in" link on refresh.

import Link from "next/link";
import { useRouter } from "next/navigation";

import { logout } from "@/lib/api";
import { useCurrentUser } from "@/lib/useAuth";

export default function NavBar() {
  const { user, loading } = useCurrentUser();
  const router = useRouter();

  async function handleLogout() {
    await logout();
    router.push("/login");
  }

  return (
    <header className="border-b border-[#332C25] bg-[#161310]">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-4 px-4 py-4">
        <Link href="/" className="font-serif text-lg text-[#F3EDE2]">
          Gem &amp; Jewelry Store
        </Link>

        <nav className="flex items-center gap-6 text-sm text-[#B8AD9E]">
          <Link href="/products" className="hover:text-[#F3EDE2]">
            Catalogue
          </Link>
          <Link href="/customize" className="hover:text-[#F3EDE2]">
            Customize
          </Link>

          {!loading && user && (
            <>
              <Link
                href={user.role === "admin" ? "/admin/dashboard" : "/orders"}
                className="hover:text-[#F3EDE2]"
              >
                {user.role === "admin" ? "Dashboard" : "My Orders"}
              </Link>
              <button onClick={() => void handleLogout()} className="hover:text-[#F3EDE2]">
                Log out
              </button>
            </>
          )}

          {!loading && !user && (
            <Link href="/login" className="hover:text-[#F3EDE2]">
              Log in
            </Link>
          )}
        </nav>
      </div>
    </header>
  );
}
