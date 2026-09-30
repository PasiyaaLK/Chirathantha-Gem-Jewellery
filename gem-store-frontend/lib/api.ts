// lib/api.ts
//
// Thin fetch wrapper for the Go backend. Centralizes the base URL,
// cookie-based auth, silent access-token refresh, and error-shape
// parsing so page and component code never touches fetch() directly.
//
// AUTH MODEL: the backend now issues the access/refresh token pair as
// HttpOnly cookies (see internal/handler/auth_handler.go) rather than
// returning a token in the JSON body — so there is nothing for this
// file to store or attach manually. Every request below sends
// `credentials: "include"`, which tells the browser to send/accept
// those cookies even though the Next.js dev server (localhost:3000) and
// the Go API (localhost:8080) are different origins (same "site" —
// same registrable domain, different port — which is exactly what
// SameSite=Strict requires; see setAuthCookies' doc comment on the
// backend for the deployment caveat if you ever split these onto
// genuinely different domains).
//
// There is deliberately no getToken()/setToken() here anymore — an
// HttpOnly cookie is, by design, invisible to JavaScript, so client
// code can no longer answer "is there a token" by looking at anything
// itself. The only way to know is to ask the server (GET /api/v1/auth/me
// — see lib/useAuth.ts, which now always makes that call rather than
// checking local storage first).

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
    this.name = "ApiError";
  }
}

interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
}

function rawFetch(path: string, options: RequestOptions): Promise<Response> {
  return fetch(`${API_BASE_URL}${path}`, {
    method: options.method ?? "GET",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
  });
}

async function throwApiError(res: Response): Promise<never> {
  // Every error response from the Go API has the shape {"error": "..."}
  // — see httputil.WriteError — so this is the one place that shape
  // gets parsed, and every caller gets a real message instead of a
  // generic "request failed."
  let message = `Request failed with status ${res.status}`;
  try {
    const data = (await res.json()) as { error?: unknown };
    if (typeof data?.error === "string") message = data.error;
  } catch {
    // Response body wasn't JSON — fall back to the generic message.
  }
  throw new ApiError(res.status, message);
}

// Coalesces concurrent refresh attempts into a single in-flight
// request. Without this, several requests all hitting a 401 at once
// right as the access token expires (e.g. a page mounting and firing
// off 3-4 fetches together) would each independently try to refresh —
// and because the backend ROTATES the refresh token on every use (see
// AuthService.RefreshAccessToken), the second concurrent refresh would
// present a token the first one just revoked and get treated as replay
// / possible theft, nuking the whole session. One shared in-flight
// promise means concurrent 401s all wait on and share the SAME refresh
// attempt instead of racing each other.
let refreshInFlight: Promise<boolean> | null = null;

function attemptRefresh(): Promise<boolean> {
  if (!refreshInFlight) {
    refreshInFlight = fetch(`${API_BASE_URL}/api/v1/auth/refresh`, {
      method: "POST",
      credentials: "include",
    })
      .then((res) => res.ok)
      .catch(() => false)
      .finally(() => {
        refreshInFlight = null;
      });
  }
  return refreshInFlight;
}

export async function apiFetch<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  let res = await rawFetch(path, options);

  // A 401 on any endpoint OTHER than the auth endpoints themselves most
  // likely means the short-lived (15 min by default) access token has
  // expired — try ONE silent refresh and retry the original request
  // once. Never attempt this for the auth endpoints themselves: a
  // failed login legitimately returns 401 too, and refreshing there
  // would just be wrong (and retrying /auth/refresh's own 401 would
  // recurse).
  const isAuthEndpoint = path.startsWith("/api/v1/auth/");
  if (res.status === 401 && !isAuthEndpoint) {
    const refreshed = await attemptRefresh();
    if (refreshed) {
      res = await rawFetch(path, options);
    }
  }

  if (!res.ok) {
    await throwApiError(res);
  }

  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

/**
 * Revokes the current session server-side and clears both auth cookies.
 * There is no client-side equivalent anymore (unlike the old
 * localStorage clearToken()) — only the server can clear an HttpOnly
 * cookie, via the Set-Cookie headers on this response.
 */
export async function logout(): Promise<void> {
  await fetch(`${API_BASE_URL}/api/v1/auth/logout`, {
    method: "POST",
    credentials: "include",
  });
}
