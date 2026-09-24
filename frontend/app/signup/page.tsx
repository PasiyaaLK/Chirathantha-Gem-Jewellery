"use client";

// app/signup/page.tsx

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { apiFetch, ApiError, setToken } from "@/lib/api";
import { buttonPrimaryClass, inputClass, labelClass, pageShellClass } from "@/lib/styles";
import type { AuthResponse } from "@/lib/types";

// Mirrors AuthService.SignUp's server-side rule — checked here only for
// immediate feedback; the server's check is the one that actually
// matters and is never bypassable from the client.
const MIN_PASSWORD_LENGTH = 8;

export default function SignupPage() {
  const router = useRouter();
  const [fullName, setFullName] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);

    if (password.length < MIN_PASSWORD_LENGTH) {
      setError(`Password must be at least ${MIN_PASSWORD_LENGTH} characters.`);
      return;
    }
    if (password !== confirmPassword) {
      setError("Passwords don't match.");
      return;
    }

    setSubmitting(true);
    try {
      // The real backend route is POST /api/v1/auth/signup (see
      // internal/handler/router.go) — used here rather than /register.
      const result = await apiFetch<AuthResponse>("/api/v1/auth/signup", {
        method: "POST",
        body: {
          full_name: fullName.trim(),
          email: email.trim(),
          password,
          phone: phone.trim() || undefined,
        },
      });
      setToken(result.token);
      // Public signup always creates a 'customer' account server-side
      // (AuthService.SignUp hardcodes the role — see Step 3) — no role
      // branch needed here, unlike the login page.
      router.push("/orders");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Something went wrong creating your account.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className={`${pageShellClass} flex items-center justify-center px-4 py-16`}>
      <div className="w-full max-w-sm">
        <h1 className="font-serif text-3xl">Create your account</h1>
        <p className="mt-2 text-[#B8AD9E]">Save your custom designs and track orders.</p>

        <form onSubmit={handleSubmit} className="mt-8 space-y-5">
          <div>
            <label className={labelClass} htmlFor="fullName">
              Full name
            </label>
            <input
              id="fullName"
              required
              className={inputClass}
              value={fullName}
              onChange={(e) => setFullName(e.target.value)}
            />
          </div>

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
            <label className={labelClass} htmlFor="phone">
              Phone <span className="text-[#6E6459]">(optional)</span>
            </label>
            <input
              id="phone"
              type="tel"
              autoComplete="tel"
              className={inputClass}
              value={phone}
              onChange={(e) => setPhone(e.target.value)}
            />
          </div>

          <div>
            <label className={labelClass} htmlFor="password">
              Password
            </label>
            <input
              id="password"
              type="password"
              autoComplete="new-password"
              required
              minLength={MIN_PASSWORD_LENGTH}
              className={inputClass}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>

          <div>
            <label className={labelClass} htmlFor="confirmPassword">
              Confirm password
            </label>
            <input
              id="confirmPassword"
              type="password"
              autoComplete="new-password"
              required
              className={inputClass}
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
            />
          </div>

          {error && <p className="text-sm text-[#D96C55]">{error}</p>}

          <button type="submit" disabled={submitting} className={`${buttonPrimaryClass} w-full`}>
            {submitting ? "Creating account…" : "Create account"}
          </button>
        </form>

        <p className="mt-6 text-center text-sm text-[#B8AD9E]">
          Already have an account?{" "}
          <Link href="/login" className="text-[#C9A46A] hover:underline">
            Log in
          </Link>
        </p>
      </div>
    </div>
  );
}
