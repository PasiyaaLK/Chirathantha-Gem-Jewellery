package handler

import (
	"net/http"

	"gemstore/internal/httputil"
	"gemstore/internal/service"
)

type PricingHandler struct {
	svc *service.PricingService
}

func NewPricingHandler(svc *service.PricingService) *PricingHandler {
	return &PricingHandler{svc: svc}
}

// Catalog handles GET /api/v1/pricing/catalog — public, read-only. A
// frontend fetches this once and computes its own live price estimate
// locally using these exact numbers, so "client-side pricing that
// matches the server's rules" is enforced by construction (same catalog,
// evaluated twice) rather than by two people remembering to keep a Go
// file and a TypeScript file in sync by hand.
func (h *PricingHandler) Catalog(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, h.svc.Catalog())
}
