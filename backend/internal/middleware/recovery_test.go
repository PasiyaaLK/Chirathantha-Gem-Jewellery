package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPanicRecovery_ReturnsGenericErrorAndDoesNotCrash(t *testing.T) {
	panicky := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(`sensitive: pgx error "password authentication failed for user \"admin\" at /home/deploy/gemstore/internal/repository/order_repository.go:42"`)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/whatever", nil)

	// The real assertion here is simply that this line doesn't crash the
	// test process — PanicRecovery is what stands between a handler
	// panic and the whole server going down.
	PanicRecovery(panicky).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "internal server error") {
		t.Errorf("body should contain the generic message, got: %s", body)
	}

	for _, leaked := range []string{"password authentication failed", "pgx error", "order_repository.go", "/home/deploy"} {
		if strings.Contains(body, leaked) {
			t.Errorf("response body leaked internal detail %q — full body: %s", leaked, body)
		}
	}
}

func TestPanicRecovery_PassesThroughWhenNoPanic(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fine"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	PanicRecovery(ok).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "fine" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "fine")
	}
}
