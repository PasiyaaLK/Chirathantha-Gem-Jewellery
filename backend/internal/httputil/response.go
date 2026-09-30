// Package httputil holds small helpers shared by every handler so response
// shapes stay consistent across the API instead of each handler rolling
// its own JSON encoding.
package httputil

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

type errorBody struct {
	Error string `json:"error"`
}

// WriteJSON encodes v as JSON with the given status code. Encoding errors
// are logged, not panicked on — by the time we're writing the body,
// there's nothing useful left to do but note it happened.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("httputil: failed to encode JSON response", "error", err)
	}
}

// WriteError writes a consistent {"error": "..."} body. The message
// passed here is always safe to show a client — never pass a raw
// internal/database error string to this function; log the real error
// server-side and pass a clean message instead.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, errorBody{Error: message})
}

// DecodeJSON decodes r.Body as JSON into dst. On success it returns
// true and writes nothing. On failure it writes the appropriate error
// response itself — 413 if the body exceeded the cap applied by
// middleware.MaxBodySize, 400 for any other malformed JSON — and
// returns false, so callers just do:
//
//	if !httputil.DecodeJSON(w, r, &req) { return }
//
// An empty body (ContentLength == 0) is treated as success with dst
// left unmodified, for endpoints where the body is optional (e.g. the
// admin approve endpoint, which accepts notes but doesn't require them).
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.ContentLength == 0 {
		return true
	}

	err := json.NewDecoder(r.Body).Decode(dst)
	if err == nil {
		return true
	}

	// http.MaxBytesReader (wired up via middleware.MaxBodySize) returns
	// a *http.MaxBytesError specifically for this case since Go 1.19 —
	// a typed error to check with errors.As, not a string to pattern-match.
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		WriteError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return false
	}

	WriteError(w, http.StatusBadRequest, "malformed JSON body")
	return false
}
