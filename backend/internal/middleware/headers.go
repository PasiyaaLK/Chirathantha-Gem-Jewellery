package middleware

import "net/http"

// SecurityHeaders sets a standard set of defensive response headers on
// every response — cheap, broadly-applicable protection against a
// handful of well-known browser-side attack classes (clickjacking,
// MIME-sniffing, protocol downgrade). None of these replace proper
// input validation or auth; they cost nothing and there's no reason not
// to have them.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()

		// Clickjacking: never allow this API's responses to be framed.
		// Mostly relevant to any HTML this API might ever serve (e.g. a
		// default error page from a proxy in front of it) — a JSON
		// response can't meaningfully be "framed" either way, but this
		// costs nothing and covers that case.
		h.Set("X-Frame-Options", "DENY")

		// Stop browsers from guessing content types and executing, say,
		// an uploaded image as JS/HTML based on sniffed content rather
		// than the declared Content-Type.
		h.Set("X-Content-Type-Options", "nosniff")

		// Don't leak the full referring URL to third-party sites linked
		// from any HTML this API serves — send only the origin on
		// cross-origin navigation, the full URL only same-origin.
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// HSTS: once a browser has seen this over HTTPS, force HTTPS for
		// a year, including subdomains. Per RFC 6797, browsers only
		// honor this header on responses actually delivered over HTTPS —
		// sending it over plain HTTP (as local dev does) is inert, not
		// harmful, so this needs no environment-conditional logic.
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// CSP: this is a JSON API, not an HTML-serving app, so the
		// policy is deliberately "nothing is allowed to load" rather
		// than the nuanced script-src/style-src allow-lists a real
		// webpage CSP needs for its own assets/CDNs. default-src 'none'
		// is safe specifically because this API never returns HTML/JS
		// for a browser to execute — copying a typical webpage CSP onto
		// a JSON API would be both wrong (allow-listing origins that
		// don't apply here) and pointless (there's no page to protect).
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

		next.ServeHTTP(w, r)
	})
}
