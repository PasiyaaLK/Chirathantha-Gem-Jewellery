// lib/api.ts
//
// Thin fetch wrapper for the Go backend. Centralizes the base URL, JSON
// handling, auth header injection, and error-shape parsing so page and
// component code never touches fetch() directly.

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

// --- Token storage -----------------------------------------------------
//
// Reads/writes the JWT from localStorage under a fixed key. That's the
// simplest thing that works before a real login page/session flow
// exists in this frontend — but localStorage is readable by any script
// on the page, which makes it vulnerable to token theft via XSS. Once
// there's a login page, the recommended upgrade is a Next.js Route
// Handler that sets an httpOnly cookie instead, so the token never
// touches client-side JS at all. Swap getToken()/setToken()'s bodies for
// that and nothing else in this file (or any caller) needs to change.

const TOKEN_STORAGE_KEY = "gemstore_token";

export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return window.localStorage.getItem(TOKEN_STORAGE_KEY);
}

export function setToken(token: string): void {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(TOKEN_STORAGE_KEY, token);
}

export function clearToken(): void {
  if (typeof window === "undefined") return;
  window.localStorage.removeItem(TOKEN_STORAGE_KEY);
}

// --- Error type ----------------------------------------------------------

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
    this.name = "ApiError";
  }
}

// --- Request helper --------------------------------------------------------

interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
  /** Attach `Authorization: Bearer <token>` if a token is stored. */
  auth?: boolean;
}

export async function apiFetch<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };

  if (options.auth) {
    const token = getToken();
    if (token) headers.Authorization = `Bearer ${token}`;
  }

  const res = await fetch(`${API_BASE_URL}${path}`, {
    method: options.method ?? "GET",
    headers,
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
  });

  // Every error response from the Go API has the shape {"error": "..."}
  // — see httputil.WriteError — so this is the one place that shape
  // gets parsed, and every caller gets a real message instead of a
  // generic "request failed."
  if (!res.ok) {
    let message = `Request failed with status ${res.status}`;
    try {
      const data = (await res.json()) as { error?: unknown };
      if (typeof data?.error === "string") message = data.error;
    } catch {
      // Response body wasn't JSON — fall back to the generic message.
    }
    throw new ApiError(res.status, message);
  }

  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}
