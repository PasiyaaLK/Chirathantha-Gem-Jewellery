"use client";

// lib/useAuth.ts
//
// Fetches the current user's profile via GET /api/v1/auth/me using
// whatever token is in storage. Used by any page that needs to know who
// (or whether) someone is logged in — the admin dashboard's role guard
// and the order-tracking page's "please log in" redirect both depend on
// this instead of duplicating their own token-checking logic.

import { useEffect, useState } from "react";

import { apiFetch, ApiError, getToken } from "./api";
import type { User } from "./types";

export interface AuthState {
  user: User | null;
  /**
   * True until the check has settled one way or the other. Callers
   * MUST wait for this to go false before deciding "not logged in" —
   * collapsing "still checking" and "definitely not logged in" into one
   * state is how a protected page ends up flashing its content (or
   * redirecting a legitimately logged-in user) while the token check is
   * still in flight.
   */
  loading: boolean;
  error: string | null;
}

export function useCurrentUser(): AuthState {
  const [state, setState] = useState<AuthState>({ user: null, loading: true, error: null });

  useEffect(() => {
    if (!getToken()) {
      setState({ user: null, loading: false, error: null });
      return;
    }

    let cancelled = false;
    apiFetch<User>("/api/v1/auth/me", { auth: true })
      .then((user) => {
        if (!cancelled) setState({ user, loading: false, error: null });
      })
      .catch((err) => {
        if (!cancelled) {
          setState({
            user: null,
            loading: false,
            error: err instanceof ApiError ? err.message : "Failed to load your account.",
          });
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return state;
}
