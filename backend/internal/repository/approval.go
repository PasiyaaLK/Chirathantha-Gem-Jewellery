package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gemstore/internal/models"
)

// ErrOrderNotPending is returned when an approve/decline is attempted on
// an order that either doesn't exist, isn't a custom order, or has
// already been decided. The status filter that produces this is baked
// into the UPDATE's WHERE clause (see DecideCustomOrder) rather than
// implemented as a separate SELECT-then-UPDATE, so two admins deciding
// the same order at the same instant can't both "win" — only the first
// UPDATE matches a row; the second gets zero rows and this error.
var ErrOrderNotPending = errors.New("repository: order is not awaiting review")

type ApprovalRepository struct {
	pool *pgxpool.Pool
}

func NewApprovalRepository(pool *pgxpool.Pool) *ApprovalRepository {
	return &ApprovalRepository{pool: pool}
}

// ListPending returns every custom order currently awaiting owner review,
// joined with its customization spec, line-item quantity/price, and the
// customer's contact details — everything the admin review screen needs
// in one call.
func (r *ApprovalRepository) ListPending(ctx context.Context) ([]models.PendingCustomOrder, error) {
	const query = `
		SELECT
			o.id, o.user_id, o.status, o.total_amount, o.shipping_address, o.created_at,
			u.full_name, u.email, u.phone,
			oi.quantity, oi.unit_price,
			c.id, c.user_id, c.category, c.gender, c.metal_type, c.gemstone_type,
			c.size, c.engraving_text, c.preview_image_url, c.estimated_price,
			c.created_at, c.updated_at
		FROM orders o
		JOIN order_items oi ON oi.order_id = o.id
		JOIN customizations c ON c.id = oi.customization_id
		JOIN users u ON u.id = o.user_id
		WHERE o.order_type = 'custom' AND o.status = 'pending_review'
		ORDER BY o.created_at ASC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("repository: list pending custom orders: %w", err)
	}
	defer rows.Close()

	var results []models.PendingCustomOrder
	for rows.Next() {
		var p models.PendingCustomOrder
		if err := rows.Scan(
			&p.OrderID, &p.UserID, &p.Status, &p.TotalAmount, &p.ShippingAddress, &p.CreatedAt,
			&p.CustomerName, &p.CustomerEmail, &p.CustomerPhone,
			&p.Quantity, &p.UnitPrice,
			&p.Customization.ID, &p.Customization.UserID, &p.Customization.Category,
			&p.Customization.Gender, &p.Customization.MetalType, &p.Customization.GemstoneType,
			&p.Customization.Size, &p.Customization.EngravingText,
			&p.Customization.PreviewImageURL, &p.Customization.EstimatedPrice,
			&p.Customization.CreatedAt, &p.Customization.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("repository: scan pending custom order: %w", err)
		}
		results = append(results, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: iterate pending custom orders: %w", err)
	}

	return results, nil
}

// ApprovalResult bundles the updated order with the customer's contact
// details, so the caller (service layer) has everything it needs to fire
// a notification without a second round trip to the database.
type ApprovalResult struct {
	Order    models.Order
	Customer models.CustomerContact
}

// DecideCustomOrder records an admin's confirm/decline decision on a
// custom order. All three effects — the order_approvals audit row, the
// orders.status update, and the customer-contact lookup — happen inside
// one transaction, so a mid-write failure can never leave an approval
// logged against an order whose status didn't actually change (or vice
// versa).
func (r *ApprovalRepository) DecideCustomOrder(
	ctx context.Context,
	orderID uuid.UUID,
	adminID uuid.UUID,
	decision models.ApprovalDecision,
	notes *string,
) (*ApprovalResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("repository: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	newStatus := models.StatusDeclined
	if decision == models.DecisionConfirmed {
		newStatus = models.StatusConfirmed
	}

	// The WHERE clause doubles as the concurrency guard described above:
	// it only matches (and therefore only updates) a row that is still
	// 'pending_review'.
	const updateOrder = `
		UPDATE orders
		SET status = $1
		WHERE id = $2 AND order_type = 'custom' AND status = 'pending_review'
		RETURNING id, user_id, order_type, status, total_amount, created_at, updated_at`

	var order models.Order
	err = tx.QueryRow(ctx, updateOrder, newStatus, orderID).Scan(
		&order.ID, &order.UserID, &order.OrderType, &order.Status,
		&order.TotalAmount, &order.CreatedAt, &order.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrOrderNotPending
	}
	if err != nil {
		return nil, fmt.Errorf("repository: update order status: %w", err)
	}

	const insertApproval = `
		INSERT INTO order_approvals (order_id, admin_id, decision, notes)
		VALUES ($1, $2, $3, $4)`
	if _, err := tx.Exec(ctx, insertApproval, orderID, adminID, decision, notes); err != nil {
		return nil, fmt.Errorf("repository: insert order approval: %w", err)
	}

	const contactQuery = `
		SELECT id, full_name, email, phone FROM users WHERE id = $1`
	var contact models.CustomerContact
	if err := tx.QueryRow(ctx, contactQuery, order.UserID).Scan(
		&contact.UserID, &contact.FullName, &contact.Email, &contact.Phone,
	); err != nil {
		return nil, fmt.Errorf("repository: fetch customer contact: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("repository: commit decision tx: %w", err)
	}

	// order_items for this order are immutable by this point (fixed at
	// creation time; this transaction only ever touches orders.status
	// and order_approvals), so fetching them via the pool after commit
	// — rather than inside the transaction above — is safe and simpler.
	// Without this, the notification email/SMS built from this Order
	// would have an empty item list: the UPDATE...RETURNING above only
	// ever populated the order's own columns, never its line items.
	itemsByOrder, err := fetchItemsWithDetails(ctx, r.pool, []uuid.UUID{order.ID})
	if err != nil {
		return nil, err
	}
	order.Items = itemsByOrder[order.ID]

	return &ApprovalResult{Order: order, Customer: contact}, nil
}
