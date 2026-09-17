// Package models contains the core domain types shared across the
// repository, service, and handler layers. These map closely to the
// tables in migrations/0001_init_schema.sql.
package models

import (
	"time"

	"github.com/google/uuid"
)

// --- Enum-like string types --------------------------------------------
// Postgres enforces the real constraint via native ENUM types; these Go
// types just give us compile-time safety and a single place to validate
// incoming values before they ever reach a query.

type UserRole string

const (
	RoleCustomer UserRole = "customer"
	RoleAdmin    UserRole = "admin"
)

type JewelryCategory string

const (
	CategoryRing     JewelryCategory = "ring"
	CategoryBracelet JewelryCategory = "bracelet"
	CategoryNecklace JewelryCategory = "necklace"
)

func (c JewelryCategory) Valid() bool {
	switch c {
	case CategoryRing, CategoryBracelet, CategoryNecklace:
		return true
	}
	return false
}

type Gender string

const (
	GenderMen    Gender = "men"
	GenderWomen  Gender = "women"
	GenderUnisex Gender = "unisex"
)

func (g Gender) Valid() bool {
	switch g {
	case GenderMen, GenderWomen, GenderUnisex:
		return true
	}
	return false
}

type OrderType string

const (
	OrderTypeStandard OrderType = "standard"
	OrderTypeCustom   OrderType = "custom"
)

type OrderStatus string

const (
	StatusCart          OrderStatus = "cart"
	StatusPendingReview OrderStatus = "pending_review"
	StatusConfirmed     OrderStatus = "confirmed"
	StatusDeclined      OrderStatus = "declined"
	StatusInProgress    OrderStatus = "in_progress"
	StatusShipped       OrderStatus = "shipped"
	StatusDelivered     OrderStatus = "delivered"
	StatusCancelled     OrderStatus = "cancelled"
)

type ApprovalDecision string

const (
	DecisionConfirmed ApprovalDecision = "confirmed"
	DecisionDeclined  ApprovalDecision = "declined"
)

// --- Core entities -------------------------------------------------------

type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // never serialized to JSON
	FullName     string    `json:"full_name"`
	Phone        *string   `json:"phone,omitempty"`
	Role         UserRole  `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Product struct {
	ID            uuid.UUID       `json:"id"`
	SKU           string          `json:"sku"`
	Name          string          `json:"name"`
	Description   *string         `json:"description,omitempty"`
	Category      JewelryCategory `json:"category"`
	Gender        Gender          `json:"gender"`
	MetalType     *string         `json:"metal_type,omitempty"`
	GemstoneType  *string         `json:"gemstone_type,omitempty"`
	BasePrice     float64         `json:"base_price"`
	StockQuantity int             `json:"stock_quantity"`
	ImageURL      *string         `json:"image_url,omitempty"`
	IsActive      bool            `json:"is_active"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// ProductFilter carries optional catalogue filters (Section 5.1: filters
// for category and gender). Zero values mean "no filter".
type ProductFilter struct {
	Category *JewelryCategory
	Gender   *Gender
	Limit    int
	Offset   int
}

type Customization struct {
	ID              uuid.UUID       `json:"id"`
	UserID          uuid.UUID       `json:"user_id"`
	Category        JewelryCategory `json:"category"`
	Gender          Gender          `json:"gender"`
	MetalType       string          `json:"metal_type"`
	GemstoneType    *string         `json:"gemstone_type,omitempty"`
	Size            *string         `json:"size,omitempty"`
	EngravingText   *string         `json:"engraving_text,omitempty"`
	PreviewImageURL *string         `json:"preview_image_url,omitempty"`
	EstimatedPrice  *float64        `json:"estimated_price,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type Order struct {
	ID              uuid.UUID   `json:"id"`
	UserID          uuid.UUID   `json:"user_id"`
	OrderType       OrderType   `json:"order_type"`
	Status          OrderStatus `json:"status"`
	ShippingAddress *string     `json:"shipping_address,omitempty"` // raw JSONB text
	TotalAmount     float64     `json:"total_amount"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`

	// Populated for detail views; empty on list views.
	Items []OrderItem `json:"items,omitempty"`
}

type OrderItem struct {
	ID              uuid.UUID  `json:"id"`
	OrderID         uuid.UUID  `json:"order_id"`
	ProductID       *uuid.UUID `json:"product_id,omitempty"`
	CustomizationID *uuid.UUID `json:"customization_id,omitempty"`
	Quantity        int        `json:"quantity"`
	UnitPrice       float64    `json:"unit_price"`
	CreatedAt       time.Time  `json:"created_at"`
}

type OrderApproval struct {
	ID        uuid.UUID        `json:"id"`
	OrderID   uuid.UUID        `json:"order_id"`
	AdminID   uuid.UUID        `json:"admin_id"`
	Decision  ApprovalDecision `json:"decision"`
	Notes     *string          `json:"notes,omitempty"`
	DecidedAt time.Time        `json:"decided_at"`
}

// --- Admin / approval workflow types ---------------------------------------

// CustomerContact is the minimal set of fields needed to notify a
// customer about a decision on their order. Deliberately separate from
// User so a repository method that only needs contact details doesn't
// have to select (or accidentally expose) password_hash or role.
type CustomerContact struct {
	UserID   uuid.UUID `json:"user_id"`
	FullName string    `json:"full_name"`
	Email    string    `json:"email"`
	Phone    *string   `json:"phone,omitempty"`
}

// PendingCustomOrder is the flattened, read-optimized view backing the
// admin "orders pending review" list and review screen (Section 5.2:
// "showing the customer's exact selections and live-preview image").
// It's a query-shaped DTO, not a table — joins orders + order_items +
// customizations + users into one row per pending order.
type PendingCustomOrder struct {
	OrderID       uuid.UUID     `json:"order_id"`
	UserID        uuid.UUID     `json:"user_id"`
	Status        OrderStatus   `json:"status"`
	TotalAmount   float64       `json:"total_amount"`
	CreatedAt     time.Time     `json:"created_at"`
	CustomerName  string        `json:"customer_name"`
	CustomerEmail string        `json:"customer_email"`
	CustomerPhone *string       `json:"customer_phone,omitempty"`
	Quantity      int           `json:"quantity"`
	UnitPrice     float64       `json:"unit_price"`
	Customization Customization `json:"customization"`
}

// --- Request DTOs ----------------------------------------------------------

// DecisionRequest is the payload for the admin approve/decline endpoints.
type DecisionRequest struct {
	Notes *string `json:"notes,omitempty"`
}

// SignUpRequest is the payload for POST /api/v1/auth/signup. Note there
// is no role field — public signup always creates a 'customer' account;
// see cmd/seedadmin for creating the store owner's admin account.
type SignUpRequest struct {
	Email    string  `json:"email"`
	Password string  `json:"password"`
	FullName string  `json:"full_name"`
	Phone    *string `json:"phone,omitempty"`
}

// LoginRequest is the payload for POST /api/v1/auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse is returned by both signup and login: a bearer token plus
// the profile of the account it belongs to.
type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// --- Pricing ---------------------------------------------------------------

// PricingBreakdown documents how a custom order's price was computed,
// returned alongside the order so the frontend can show its own
// estimate was (or wasn't) correct, rather than just a final number.
// See internal/service/pricing.go for the calculation itself.
type PricingBreakdown struct {
	CategoryBase    float64 `json:"category_base"`
	MetalMultiplier float64 `json:"metal_multiplier"`
	AfterMetal      float64 `json:"after_metal"`
	GemstoneFee     float64 `json:"gemstone_fee"`
	EngravingFee    float64 `json:"engraving_fee"`
	Total           float64 `json:"total"`
}

// PricingCatalog is the raw lookup data behind PricingBreakdown, exposed
// via GET /api/v1/pricing/catalog so a frontend can compute its own
// live estimate against the exact same numbers instead of a hand-copied
// (and driftable) second source of truth.
type PricingCatalog struct {
	CategoryBasePrices  map[string]float64 `json:"category_base_prices"`
	MetalMultipliers    map[string]float64 `json:"metal_multipliers"`
	GemstoneFees        map[string]float64 `json:"gemstone_fees"`
	EngravingBaseFee    float64            `json:"engraving_base_fee"`
	EngravingPerCharFee float64            `json:"engraving_per_char_fee"`
}

// CustomOrderResult is the response for POST /api/v1/orders/custom: the
// created order plus the price breakdown the server actually charged,
// so the client can reconcile it against its own local estimate.
type CustomOrderResult struct {
	Order   Order            `json:"order"`
	Pricing PricingBreakdown `json:"pricing"`
}

// CustomOrderRequest is the payload for POST /api/v1/orders/custom.
// It bundles the customization spec with the quantity/shipping info needed
// to create the order + order_item in one transaction.
type CustomOrderRequest struct {
	Category        JewelryCategory `json:"category"`
	Gender          Gender          `json:"gender"`
	MetalType       string          `json:"metal_type"`
	GemstoneType    *string         `json:"gemstone_type,omitempty"`
	Size            *string         `json:"size,omitempty"`
	EngravingText   *string         `json:"engraving_text,omitempty"`
	PreviewImageURL *string         `json:"preview_image_url,omitempty"`
	EstimatedPrice  float64         `json:"estimated_price"`
	Quantity        int             `json:"quantity"`
	ShippingAddress *string         `json:"shipping_address,omitempty"`
}
