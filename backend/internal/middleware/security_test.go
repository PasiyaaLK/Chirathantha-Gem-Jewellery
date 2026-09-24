package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestIPRateLimiter_AllowsBurstThenBlocks(t *testing.T) {
	// burst=3: the token bucket starts full, so the first 3 calls to
	// Allow() succeed immediately with no time passing — this is
	// deterministic and needs no sleeping/clock injection, unlike
	// testing the refill rate would.
	limiter := NewIPRateLimiter(rate.Every(time.Minute), 3)

	for i := 1; i <= 3; i++ {
		if !limiter.allow("1.2.3.4") {
			t.Fatalf("request %d: expected allowed (within burst), got blocked", i)
		}
	}
	if limiter.allow("1.2.3.4") {
		t.Error("4th request within the burst window: expected blocked, got allowed")
	}
}

func TestIPRateLimiter_TracksEachIPIndependently(t *testing.T) {
	limiter := NewIPRateLimiter(rate.Every(time.Minute), 1)

	if !limiter.allow("1.1.1.1") {
		t.Fatal("first request from 1.1.1.1 should be allowed")
	}
	if limiter.allow("1.1.1.1") {
		t.Error("second request from 1.1.1.1 should be blocked (burst=1)")
	}
	if !limiter.allow("2.2.2.2") {
		t.Error("first request from a DIFFERENT IP should be allowed — limiters are per-IP")
	}
}

func TestRateLimit_Middleware_Returns429WhenExceeded(t *testing.T) {
	limiter := NewIPRateLimiter(rate.Every(time.Minute), 1)
	handler := RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "5.6.7.8:54321"

	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request: status = %d, want 200", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: status = %d, want 429", rec2.Code)
	}
}

func TestMaxBodySize_RejectsOversizedBody(t *testing.T) {
	const limit = 10 // bytes

	handler := MaxBodySize(limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		_, err := r.Body.Read(buf)
		if err != nil && err.Error() != "EOF" {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("this body is way more than ten bytes long"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413 for a body exceeding the %d-byte cap", rec.Code, limit)
	}
}
