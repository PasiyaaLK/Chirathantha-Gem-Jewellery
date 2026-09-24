package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"gemstore/internal/httputil"
)

// --- Rate limiting -------------------------------------------------------

// ipRateLimiterEntry pairs a per-IP token bucket with when it was last
// used, so the background sweep in cleanupLoop knows which entries are
// stale.
type ipRateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimiter hands out a golang.org/x/time/rate token bucket per
// client IP, so one abusive IP hitting a limit doesn't affect anyone
// else's. Entries for IPs that haven't been seen in a while are swept
// periodically — without this, the map would grow forever (one entry
// per distinct IP ever seen), which is itself a memory-exhaustion DoS
// vector on a public endpoint.
type IPRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipRateLimiterEntry
	rate     rate.Limit
	burst    int
}

// NewIPRateLimiter builds a limiter allowing burst immediate requests
// per IP, refilling at r events/sec thereafter, and starts its
// background cleanup goroutine. For "N attempts per window" (e.g. "5
// per minute"), construct r with rate.Every(window/N) — see
// handler.NewRouter for how the auth endpoints use this.
func NewIPRateLimiter(r rate.Limit, burst int) *IPRateLimiter {
	l := &IPRateLimiter{
		limiters: make(map[string]*ipRateLimiterEntry),
		rate:     r,
		burst:    burst,
	}
	go l.cleanupLoop()
	return l
}

func (l *IPRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, exists := l.limiters[ip]
	if !exists {
		entry = &ipRateLimiterEntry{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.limiters[ip] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter.Allow()
}

// cleanupLoop periodically evicts limiters for IPs not seen recently.
// Runs for the lifetime of the process — IPRateLimiter instances are
// created once at startup (see handler.NewRouter) and live as long as
// the server does, so there's no corresponding stop/cancel; the
// goroutine exits only when the process does.
func (l *IPRateLimiter) cleanupLoop() {
	const (
		sweepInterval = 10 * time.Minute
		staleAfter    = 15 * time.Minute
	)
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()

	for range ticker.C {
		l.mu.Lock()
		for ip, entry := range l.limiters {
			if time.Since(entry.lastSeen) > staleAfter {
				delete(l.limiters, ip)
			}
		}
		l.mu.Unlock()
	}
}

// RateLimit rejects requests from an IP exceeding limiter's rate with
// 429 Too Many Requests. Intended for specific brute-force-prone routes
// (login, signup) via chi's r.With(...), not applied globally.
//
// IMPORTANT DEPLOYMENT NOTE: this keys on the request's RemoteAddr,
// which chi's middleware.RealIP (if it runs earlier in the chain, as it
// does in handler.NewRouter) rewrites based on the X-Forwarded-For /
// X-Real-IP headers. That rewrite is only trustworthy when there's
// actually a reverse proxy or load balancer in front of this server
// that sets those headers itself and strips any client-supplied values
// — otherwise any client can set X-Forwarded-For to an arbitrary value
// and get a fresh rate-limit bucket on every request, defeating this
// entirely. If this server is ever directly internet-facing without
// such a proxy, remove middleware.RealIP from the chain (or configure
// it with chi's trusted-proxy options) so this falls back to the
// connection's actual RemoteAddr.
func RateLimit(limiter *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if !limiter.allow(ip) {
				httputil.WriteError(w, http.StatusTooManyRequests, "too many requests — please try again shortly")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr wasn't in host:port form — fall back to using it
		// as-is rather than failing the request over a parsing detail.
		return r.RemoteAddr
	}
	return host
}

// --- Body size limiting ---------------------------------------------------

// MaxBodySize wraps every request body in an http.MaxBytesReader capped
// at maxBytes, applied globally (see handler.NewRouter) as a blanket
// defense against a client streaming an enormous body to exhaust server
// memory — independent of whatever a specific handler's own validation
// does or doesn't check. Reading past the cap makes the next Read (and
// so json.Decode — see httputil.DecodeJSON, which specifically detects
// this case) return an *http.MaxBytesError.
func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
