package httputil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSON_Success(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"test"}`))
	req.ContentLength = int64(len(`{"name":"test"}`))
	rec := httptest.NewRecorder()

	var dst struct {
		Name string `json:"name"`
	}
	ok := DecodeJSON(rec, req, &dst)

	if !ok {
		t.Fatalf("expected success, got failure with response: %s", rec.Body.String())
	}
	if dst.Name != "test" {
		t.Errorf("Name = %q, want %q", dst.Name, "test")
	}
}

func TestDecodeJSON_EmptyBodyIsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.ContentLength = 0
	rec := httptest.NewRecorder()

	var dst struct{ Name string }
	if !DecodeJSON(rec, req, &dst) {
		t.Errorf("empty body should decode as success (for optional-body endpoints), got failure: %s", rec.Body.String())
	}
}

func TestDecodeJSON_MalformedJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{not valid json`))
	req.ContentLength = int64(len(`{not valid json`))
	rec := httptest.NewRecorder()

	var dst struct{ Name string }
	if DecodeJSON(rec, req, &dst) {
		t.Fatal("expected failure for malformed JSON, got success")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDecodeJSON_OversizedBodyReturns413(t *testing.T) {
	body := strings.Repeat("a", 100)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"`+body+`"}`))
	rec := httptest.NewRecorder()
	// Simulate what middleware.MaxBodySize does: wrap the body in a
	// MaxBytesReader capped well below what's actually being sent.
	req.Body = http.MaxBytesReader(rec, req.Body, 10)

	var dst struct{ Name string }
	if DecodeJSON(rec, req, &dst) {
		t.Fatal("expected failure for an oversized body, got success")
	}
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
}
