"use client";

// app/login/page.tsx

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { apiFetch, ApiError, setToken } from "@/lib/api";
import { buttonPrimaryClass, inputClass, labelClass, pageShellClass } from "@/lib/styles";
import type { AuthResponse } from "@/lib/types";

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);

    try {
      const result = await apiFetch<AuthResponse>("/api/v1/auth/login", {
        method: "POST",
        body: { email, password },
      });
      setToken(result.token);
      // Route by role so an admin lands on their queue and a customer
      // lands on their order history, rather than a generic landing
      // page neither of them actually wants right after logging in.
      router.push(result.user.role === "admin" ? "/admin/dashboard" : "/orders");
    } catch (err) {
      // The backend deliberately returns the identical message for "no
      // such email" and "wrong password" — see AuthService.Login's own
      // comment on why (it closes an account-enumeration timing side
      // channel). Surface it as-is; don't try to be more specific about
      // which one it was, that would defeat the point.
      setError(err instanceof ApiError ? err.message : "Something went wrong logging in.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className={`${pageShellClass} flex items-center justify-center px-4 py-16`}>
      <div className="w-full max-w-sm">
        <h1 className="font-serif text-3xl">Welcome back</h1>
        <p className="mt-2 text-[#B8AD9E]">Log in to track your orders or review custom pieces.</p>

        <form onSubmit={handleSubmit} className="mt-8 space-y-5">
          <div>
            <label className={labelClass} htmlFor="email">
              Email
            </label>
            <input
              id="email"
              type="email"
              autoComplete="email"
              required
              className={inputClass}
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </div>

          <div>
            <label className={labelClass} htmlFor="password">
              Password
            </label>
            <input
              id="password"
              type="password"
              autoComplete="current-password"
              required
              className={inputClass}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>

          {error && <p className="text-sm text-[#D96C55]">{error}</p>}

          <button type="submit" disabled={submitting} className={`${buttonPrimaryClass} w-full`}>
            {submitting ? "Logging in…" : "Log in"}
          </button>
        </form>

        <p className="mt-6 text-center text-sm text-[#B8AD9E]">
          New here?{" "}
          <Link href="/signup" className="text-[#C9A46A] hover:underline">
            Create an account
          </Link>
        </p>
      </div>
    </div>
  );
}
