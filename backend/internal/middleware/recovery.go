package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"gemstore/internal/httputil"
)

// PanicRecovery catches any panic from a handler or later middleware,
// logs the full detail server-side (including a stack trace), and
// returns a generic JSON error to the client — never the panic value, a
// stack trace, a file path, or any other internal detail.
//
// This replaces chi's middleware.Recoverer rather than stacking on top
// of it: chi's default writes a colorized stack trace to stdout (noise
// in a deployment that otherwise logs structured JSON via slog, per
// cmd/api/main.go) and sends a plain-text client response, neither of
// which matches this app's conventions. Two independent recovery
// middlewares would also be redundant — whichever sits closer to the
// handler in the chain always catches the panic first, making the
// other dead code.
//
// One inherent limitation worth knowing, not specific to this
// implementation: if a handler panics after it's already started
// writing a response body (partial JSON already flushed to the
// client), there's no way to "un-send" those bytes — the client still
// gets a truncated/malformed response regardless of what this
// middleware does. Panicking before writing anything (the normal case)
// is the scenario this actually protects.
func PanicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.ErrorContext(r.Context(), "panic recovered in HTTP handler",
					"panic", fmt.Sprintf("%v", rec),
					"stack", string(debug.Stack()),
					"method", r.Method,
					"path", r.URL.Path,
				)
				httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
