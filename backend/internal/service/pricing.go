package service

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"gemstore/internal/models"
)

// PricingService computes the authoritative price of a custom jewelry
// item from its category, metal, gemstone, and engraving choices. It is
// pure and deterministic — no I/O, no side effects, no dependency on
// anything but its inputs — so the same customization always prices the
// same way and the function is trivially unit-testable.
//
// The catalog below is hardcoded Go data, matching what was asked for
// this step. A store owner will eventually want to tune prices without
// a redeploy — a `pricing_rules` DB table (admin-editable, versioned)
// is the natural evolution once there's an admin UI for it; see the
// comment on OrderService.SubmitCustomOrder for how that would slot in
// without changing PricingService's exported shape (Calculate/Catalog).
type PricingService struct{}

func NewPricingService() *PricingService {
	return &PricingService{}
}

// categoryBasePrice is the starting price for a bare item in each
// category, before metal/gemstone/engraving are applied.
var categoryBasePrice = map[models.JewelryCategory]float64{
	models.CategoryRing:     150.00,
	models.CategoryBracelet: 220.00,
	models.CategoryNecklace: 280.00,
}

// metalMultiplier scales the category base price. Keys are matched
// case-insensitively (see normalizeKey) against models.CustomOrderRequest.MetalType.
var metalMultiplier = map[string]float64{
	"silver":      1.0,
	"white gold":  2.4,
	"yellow gold": 2.4,
	"rose gold":   2.5,
	"platinum":    3.2,
}

// gemstoneFee is a flat add-on per gemstone choice — a simplified stand-in
// for a real per-carat rate table (which would need a carat-weight input
// this step's schema doesn't have yet).
var gemstoneFee = map[string]float64{
	"cubic zirconia": 15.00,
	"amethyst":       35.00,
	"topaz":          45.00,
	"sapphire":       180.00,
	"ruby":           220.00,
	"emerald":        260.00,
	"diamond":        450.00,
}

const (
	engravingBaseFee    = 20.00 // flat fee for engraving at all
	engravingPerCharFee = 1.50  // per character on top of the base fee
)

// Calculate prices one unit of the given customization. It returns
// ErrInvalidInput — the same error type every other validation failure
// in this codebase uses — for any category/metal/gemstone name outside
// the known catalog, so the handler maps it to a 400 the same way as
// any other bad-input case: an unrecognized material name is a client
// error, not a server failure.
func (p *PricingService) Calculate(req models.CustomOrderRequest) (*models.PricingBreakdown, error) {
	base, ok := categoryBasePrice[req.Category]
	if !ok {
		return nil, ErrInvalidInput{Field: "category", Reason: "no pricing defined for this category"}
	}

	multiplier, ok := metalMultiplier[normalizeKey(req.MetalType)]
	if !ok {
		return nil, ErrInvalidInput{
			Field:  "metal_type",
			Reason: fmt.Sprintf("unrecognized metal %q — supported: %s", req.MetalType, sortedKeys(metalMultiplier)),
		}
	}

	var gemFee float64
	if req.GemstoneType != nil && strings.TrimSpace(*req.GemstoneType) != "" {
		key := normalizeKey(*req.GemstoneType)
		fee, ok := gemstoneFee[key]
		if !ok {
			return nil, ErrInvalidInput{
				Field:  "gemstone_type",
				Reason: fmt.Sprintf("unrecognized gemstone %q — supported: %s", key, sortedKeys(gemstoneFee)),
			}
		}
		gemFee = fee
	}

	afterMetal := round2(base * multiplier)

	var engravingFee float64
	if req.EngravingText != nil {
		if text := strings.TrimSpace(*req.EngravingText); text != "" {
			engravingFee = round2(engravingBaseFee + float64(len(text))*engravingPerCharFee)
		}
	}

	total := round2(afterMetal + gemFee + engravingFee)

	return &models.PricingBreakdown{
		CategoryBase:    base,
		MetalMultiplier: multiplier,
		AfterMetal:      afterMetal,
		GemstoneFee:     gemFee,
		EngravingFee:    engravingFee,
		Total:           total,
	}, nil
}

// Catalog exposes the raw pricing tables read-only, for
// GET /api/v1/pricing/catalog. A frontend fetches this once and computes
// its own live estimate against these exact numbers — "client-side
// pricing that matches the server's rules" enforced by construction
// (same numbers, evaluated twice) rather than by two people remembering
// to keep a Go file and a TypeScript file in sync by hand.
func (p *PricingService) Catalog() models.PricingCatalog {
	categories := make(map[string]float64, len(categoryBasePrice))
	for k, v := range categoryBasePrice {
		categories[string(k)] = v
	}

	return models.PricingCatalog{
		CategoryBasePrices:  categories,
		MetalMultipliers:    copyFloatMap(metalMultiplier),
		GemstoneFees:        copyFloatMap(gemstoneFee),
		EngravingBaseFee:    engravingBaseFee,
		EngravingPerCharFee: engravingPerCharFee,
	}
}

func normalizeKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func sortedKeys(m map[string]float64) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func copyFloatMap(m map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
