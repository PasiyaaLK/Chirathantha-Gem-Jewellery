package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	appmiddleware "gemstore/internal/middleware"

	"gemstore/internal/httputil"
	"gemstore/internal/models"
	"gemstore/internal/service"
)

type OrderHandler struct {
	svc *service.OrderService
}

func NewOrderHandler(svc *service.OrderService) *OrderHandler {
	return &OrderHandler{svc: svc}
}

// SubmitCustom handles POST /api/v1/orders/custom
// (Section 8, step 3: customer submits the custom design; system marks it
// "Pending Owner Review"). The caller's identity comes from the verified
// JWT — see middleware.Authenticate, mounted on this route in
// handler.NewRouter — replacing Step 1's X-User-ID placeholder.
func (h *OrderHandler) SubmitCustom(w http.ResponseWriter, r *http.Request) {
	userID, ok := appmiddleware.UserIDFromContext(r.Context())
	if !ok {
		// Only reachable if this route is ever wired without the
		// Authenticate middleware in front of it — a routing bug, not a
		// client error, hence 500 rather than 401.
		httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	var req models.CustomOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "malformed JSON body")
		return
	}

	result, err := h.svc.SubmitCustomOrder(r.Context(), userID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	// Response is {"order": {...}, "pricing": {...}} as of the pricing
	// engine — see models.CustomOrderResult — not just the bare order.
	w.Header().Set("Location", "/api/v1/orders/"+result.Order.ID.String())
	httputil.WriteJSON(w, http.StatusCreated, result)
}

// Get handles GET /api/v1/orders/{id}. Now that Authenticate runs on
// this route (see handler.NewRouter), it also enforces ownership: a
// customer can fetch their own orders; an admin can fetch any order.
// This was implicit-open in Step 1 (no auth existed yet) — closing it
// here since it's a direct, small consequence of this step's middleware,
// not a separate piece of work.
func (h *OrderHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	callerID, ok := appmiddleware.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	callerRole, _ := appmiddleware.RoleFromContext(r.Context())

	order, err := h.svc.GetOrder(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	if callerRole != models.RoleAdmin && order.UserID != callerID {
		// Same response shape as "doesn't exist" — a non-owner shouldn't
		// be able to distinguish "not yours" from "no such order" by
		// status code alone.
		httputil.WriteError(w, http.StatusNotFound, "resource not found")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, order)
}
