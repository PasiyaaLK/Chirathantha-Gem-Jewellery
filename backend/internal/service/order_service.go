package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"strings"

	"github.com/google/uuid"

	"gemstore/internal/models"
	"gemstore/internal/repository"
)

type OrderService struct {
	repo    *repository.OrderRepository
	pricing *PricingService
}

func NewOrderService(repo *repository.OrderRepository, pricing *PricingService) *OrderService {
	return &OrderService{repo: repo, pricing: pricing}
}

// SubmitCustomOrder validates the customer's customization + order request
// (Section 8, step 3: "system marks it Pending Owner Review"), prices it
// authoritatively on the server, and delegates to the repository's
// transactional insert.
//
// Validation lives here rather than in the handler so the same rules
// apply no matter what ever calls this service (HTTP today; a future
// gRPC or admin-CLI entry point tomorrow). Pricing is a separate
// PricingService dependency rather than inlined here, so it can be
// swapped for a DB-backed version later (see pricing.go) without
// touching this orchestration logic at all.
func (s *OrderService) SubmitCustomOrder(
	ctx context.Context,
	userID uuid.UUID,
	req models.CustomOrderRequest,
) (*models.CustomOrderResult, error) {
	if err := validateCustomOrderRequest(req); err != nil {
		return nil, err
	}

	breakdown, err := s.pricing.Calculate(req)
	if err != nil {
		return nil, err // already an ErrInvalidInput (unrecognized metal/gemstone/category)
	}

	// The server is the source of truth for price. Whatever the client
	// sent in req.EstimatedPrice — its own live-preview running total —
	// is never used for the actual charge; it's only compared below as a
	// diagnostic signal, then overwritten before anything is persisted.
	// Without this, a client could submit any price it likes for a real
	// gemstone/metal combination and the server would happily store it.
	if req.EstimatedPrice > 0 && math.Abs(req.EstimatedPrice-breakdown.Total) > 0.01 {
		// Both sides compute from the same catalog (the client fetches it
		// from GET /api/v1/pricing/catalog) using the same formula, so in
		// normal operation this should be an exact match. A mismatch
		// means the client's cached catalog is stale or its estimate
		// logic has drifted from the server's — worth knowing about
		// operationally, but never worth rejecting the order over, since
		// the server's number is what actually gets charged either way.
		slog.WarnContext(ctx, "custom order: client price estimate differs from server-computed price",
			"user_id", userID, "client_estimate", req.EstimatedPrice, "server_total", breakdown.Total)
	}
	req.EstimatedPrice = breakdown.Total

	order, err := s.repo.CreateCustomOrder(ctx, userID, req)
	if err != nil {
		return nil, err
	}

	return &models.CustomOrderResult{Order: *order, Pricing: *breakdown}, nil
}

func (s *OrderService) GetOrder(ctx context.Context, id uuid.UUID) (*models.Order, error) {
	return s.repo.GetByID(ctx, id)
}

func validateCustomOrderRequest(req models.CustomOrderRequest) error {
	if !req.Category.Valid() {
		return ErrInvalidInput{Field: "category", Reason: "must be one of ring, bracelet, necklace"}
	}
	if !req.Gender.Valid() {
		return ErrInvalidInput{Field: "gender", Reason: "must be one of men, women, unisex"}
	}
	if strings.TrimSpace(req.MetalType) == "" {
		return ErrInvalidInput{Field: "metal_type", Reason: "is required"}
	}
	if req.Quantity <= 0 {
		return ErrInvalidInput{Field: "quantity", Reason: "must be at least 1"}
	}
	if req.EngravingText != nil && len(*req.EngravingText) > 100 {
		return ErrInvalidInput{Field: "engraving_text", Reason: "must be 100 characters or fewer"}
	}
	// orders.shipping_address is a JSONB column (see schema.sql) — the
	// value has to actually BE valid JSON text, or Postgres rejects the
	// insert with "invalid input syntax for type json". A bare address
	// string like "123 Main St" is NOT valid JSON (it needs to be a
	// quoted JSON string, an object, etc.), so this is caught here as a
	// normal 400 rather than surfacing as a confusing 500 from the
	// repository layer. The frontend sends e.g. `{"raw": "123 Main St"}`.
	if req.ShippingAddress != nil && !json.Valid([]byte(*req.ShippingAddress)) {
		return ErrInvalidInput{
			Field:  "shipping_address",
			Reason: `must be valid JSON, e.g. {"line1": "...", "city": "..."}`,
		}
	}
	return nil
}
