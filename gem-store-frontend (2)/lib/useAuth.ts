"use client";

// lib/useAuth.ts
//
// Fetches the current user's profile via GET /api/v1/auth/me. Used by
// any page that needs to know who (or whether) someone is logged in —
// the admin dashboard's role guard and the order-tracking page's
// "please log in" redirect both depend on this instead of duplicating
// their own auth-checking logic.
//
// There's no client-side "is there a token" check anymore: the access
// token lives in an HttpOnly cookie, invisible to JS by design, so the
// only way to know if someone is logged in is to ask the server. This
// always calls /auth/me and treats any failure (no cookie, expired
// token that a silent refresh in lib/api.ts couldn't save, revoked
// session) as "not logged in" — there's no meaningfully different way
// to handle those cases from the frontend's perspective anyway.

import { useEffect, useState } from "react";

import { apiFetch, ApiError } from "./api";
import type { User } from "./types";

export interface AuthState {
  user: User | null;
  /**
   * True until the check has settled one way or the other. Callers
   * MUST wait for this to go false before deciding "not logged in" —
   * collapsing "still checking" and "definitely not logged in" into one
   * state is how a protected page ends up flashing its content (or
   * redirecting a legitimately logged-in user) while the check is
   * still in flight.
   */
  loading: boolean;
  error: string | null;
}

export function useCurrentUser(): AuthState {
  const [state, setState] = useState<AuthState>({ user: null, loading: true, error: null });

  useEffect(() => {
    let cancelled = false;

    apiFetch<User>("/api/v1/auth/me")
      .then((user) => {
        if (!cancelled) setState({ user, loading: false, error: null });
      })
      .catch((err) => {
        if (!cancelled) {
          // A 401 here just means "not logged in" — not an error worth
          // surfacing to the UI as one; only a genuine failure (network,
          // 500) goes into `error`.
          const isAuthFailure = err instanceof ApiError && err.status === 401;
          setState({
            user: null,
            loading: false,
            error: isAuthFailure ? null : "Failed to load your account.",
          });
        }
      });

    return () => {
      cancelled = true;
    };
  }, []);

  return state;
}
