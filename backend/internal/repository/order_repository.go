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

type OrderRepository struct {
	pool *pgxpool.Pool
}

func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

// CreateCustomOrder implements Section 8, steps 2-3 of the proposal:
// it persists the customer's customization spec, creates the parent order
// in 'pending_review' status, and links them with an order_item — all in
// a single transaction, so a crash midway never leaves an order without
// its customization or vice versa.
func (r *OrderRepository) CreateCustomOrder(
	ctx context.Context,
	userID uuid.UUID,
	req models.CustomOrderRequest,
) (*models.Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("repository: begin tx: %w", err)
	}
	// Rollback is a no-op once Commit has succeeded; this just guarantees
	// we never leave a transaction open on an early return.
	defer func() { _ = tx.Rollback(ctx) }()

	customizationID, err := insertCustomization(ctx, tx, userID, req)
	if err != nil {
		return nil, err
	}

	order, err := insertOrder(ctx, tx, userID, req)
	if err != nil {
		return nil, err
	}

	item, err := insertOrderItem(ctx, tx, order.ID, customizationID, req)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("repository: commit custom order tx: %w", err)
	}

	order.UserID = userID
	order.Items = []models.OrderItem{*item}
	return order, nil
}

func insertCustomization(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
	req models.CustomOrderRequest,
) (uuid.UUID, error) {
	const query = `
		INSERT INTO customizations
			(user_id, category, gender, metal_type, gemstone_type,
			 size, engraving_text, preview_image_url, estimated_price)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`

	var id uuid.UUID
	err := tx.QueryRow(ctx, query,
		userID, req.Category, req.Gender, req.MetalType, req.GemstoneType,
		req.Size, req.EngravingText, req.PreviewImageURL, req.EstimatedPrice,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("repository: insert customization: %w", err)
	}
	return id, nil
}

func insertOrder(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
	req models.CustomOrderRequest,
) (*models.Order, error) {
	const query = `
		INSERT INTO orders (user_id, order_type, status, shipping_address, total_amount)
		VALUES ($1, 'custom', 'pending_review', $2, $3)
		RETURNING id, order_type, status, total_amount, created_at, updated_at`

	totalAmount := req.EstimatedPrice * float64(req.Quantity)

	var order models.Order
	err := tx.QueryRow(ctx, query, userID, req.ShippingAddress, totalAmount).Scan(
		&order.ID, &order.OrderType, &order.Status, &order.TotalAmount,
		&order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("repository: insert order: %w", err)
	}
	return &order, nil
}

func insertOrderItem(
	ctx context.Context,
	tx pgx.Tx,
	orderID, customizationID uuid.UUID,
	req models.CustomOrderRequest,
) (*models.OrderItem, error) {
	const query = `
		INSERT INTO order_items (order_id, customization_id, quantity, unit_price)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`

	item := &models.OrderItem{
		OrderID:         orderID,
		CustomizationID: &customizationID,
		Quantity:        req.Quantity,
		UnitPrice:       req.EstimatedPrice,
	}
	err := tx.QueryRow(ctx, query, orderID, customizationID, req.Quantity, req.EstimatedPrice).
		Scan(&item.ID, &item.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("repository: insert order item: %w", err)
	}
	return item, nil
}

// GetByID fetches an order together with its line items — used to return
// the freshly created resource from the handler (201 + body), by the
// owner-or-admin detail view, and by the admin review screen.
func (r *OrderRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Order, error) {
	const orderQuery = `
		SELECT id, user_id, order_type, status, shipping_address, total_amount,
		       created_at, updated_at
		FROM orders
		WHERE id = $1`

	var order models.Order
	err := r.pool.QueryRow(ctx, orderQuery, id).Scan(
		&order.ID, &order.UserID, &order.OrderType, &order.Status,
		&order.ShippingAddress, &order.TotalAmount, &order.CreatedAt, &order.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repository: get order by id: %w", err)
	}

	itemsByOrder, err := fetchItemsWithDetails(ctx, r.pool, []uuid.UUID{order.ID})
	if err != nil {
		return nil, err
	}
	order.Items = itemsByOrder[order.ID]

	return &order, nil
}

// ListByUser returns all of a customer's orders, most recent first, each
// with its line items enriched with customization/product detail — used
// by the order-tracking page, GET /api/v1/orders. The query is scoped to
// userID at the SQL level (not filtered after the fact), so a caller can
// only ever get their own orders back, matching the ownership rule
// GetByID enforces at the handler level for a single order.
func (r *OrderRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]models.Order, error) {
	const query = `
		SELECT id, user_id, order_type, status, shipping_address, total_amount,
		       created_at, updated_at
		FROM orders
		WHERE user_id = $1
		ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("repository: list orders by user: %w", err)
	}

	var orders []models.Order
	var orderIDs []uuid.UUID
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(
			&o.ID, &o.UserID, &o.OrderType, &o.Status, &o.ShippingAddress,
			&o.TotalAmount, &o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("repository: scan order: %w", err)
		}
		orders = append(orders, o)
		orderIDs = append(orderIDs, o.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: iterate orders: %w", err)
	}

	itemsByOrder, err := fetchItemsWithDetails(ctx, r.pool, orderIDs)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		orders[i].Items = itemsByOrder[orders[i].ID]
	}

	return orders, nil
}

// fetchItemsWithDetails loads order_items for the given order IDs, then
// enriches each item with its referenced Customization or Product
// (never both — order_items_exactly_one_source enforces that) so a
// client can render "Ring — Rose Gold, Sapphire" without a second round
// trip per item.
//
// This is three simple queries (items, then customizations by ID, then
// products by ID) rather than one query with LEFT JOINs against both
// tables. A LEFT JOIN would need every joined column scanned into a
// nullable destination — including columns that are NOT NULL on their
// own table — since the *other* side of the join goes entirely NULL on
// any row where it doesn't apply. Three queries with ordinary NOT NULL
// scanning is more code but meaningfully harder to get wrong.
func fetchItemsWithDetails(
	ctx context.Context,
	pool *pgxpool.Pool,
	orderIDs []uuid.UUID,
) (map[uuid.UUID][]models.OrderItem, error) {
	result := make(map[uuid.UUID][]models.OrderItem)
	if len(orderIDs) == 0 {
		return result, nil
	}

	const itemsQuery = `
		SELECT id, order_id, product_id, customization_id, quantity, unit_price, created_at
		FROM order_items
		WHERE order_id = ANY($1)
		ORDER BY created_at`

	rows, err := pool.Query(ctx, itemsQuery, orderIDs)
	if err != nil {
		return nil, fmt.Errorf("repository: fetch order items: %w", err)
	}

	var items []models.OrderItem
	var customizationIDs, productIDs []uuid.UUID
	for rows.Next() {
		var item models.OrderItem
		if err := rows.Scan(
			&item.ID, &item.OrderID, &item.ProductID, &item.CustomizationID,
			&item.Quantity, &item.UnitPrice, &item.CreatedAt,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("repository: scan order item: %w", err)
		}
		items = append(items, item)
		if item.CustomizationID != nil {
			customizationIDs = append(customizationIDs, *item.CustomizationID)
		}
		if item.ProductID != nil {
			productIDs = append(productIDs, *item.ProductID)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: iterate order items: %w", err)
	}

	customizations, err := fetchCustomizationsByID(ctx, pool, customizationIDs)
	if err != nil {
		return nil, err
	}
	products, err := fetchProductsByID(ctx, pool, productIDs)
	if err != nil {
		return nil, err
	}

	for _, item := range items {
		if item.CustomizationID != nil {
			if c, ok := customizations[*item.CustomizationID]; ok {
				c := c // local copy — avoid every item pointing at the same loop variable
				item.Customization = &c
			}
		}
		if item.ProductID != nil {
			if p, ok := products[*item.ProductID]; ok {
				p := p
				item.Product = &p
			}
		}
		result[item.OrderID] = append(result[item.OrderID], item)
	}

	return result, nil
}

func fetchCustomizationsByID(
	ctx context.Context,
	pool *pgxpool.Pool,
	ids []uuid.UUID,
) (map[uuid.UUID]models.Customization, error) {
	out := make(map[uuid.UUID]models.Customization, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	const query = `
		SELECT id, user_id, category, gender, metal_type, gemstone_type,
		       size, engraving_text, preview_image_url, estimated_price,
		       created_at, updated_at
		FROM customizations
		WHERE id = ANY($1)`

	rows, err := pool.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("repository: fetch customizations by id: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var c models.Customization
		if err := rows.Scan(
			&c.ID, &c.UserID, &c.Category, &c.Gender, &c.MetalType, &c.GemstoneType,
			&c.Size, &c.EngravingText, &c.PreviewImageURL, &c.EstimatedPrice,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("repository: scan customization: %w", err)
		}
		out[c.ID] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: iterate customizations: %w", err)
	}
	return out, nil
}

func fetchProductsByID(
	ctx context.Context,
	pool *pgxpool.Pool,
	ids []uuid.UUID,
) (map[uuid.UUID]models.Product, error) {
	out := make(map[uuid.UUID]models.Product, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	const query = `
		SELECT id, sku, name, description, category, gender, metal_type,
		       gemstone_type, base_price, stock_quantity, image_url,
		       is_active, created_at, updated_at
		FROM products
		WHERE id = ANY($1)`

	rows, err := pool.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("repository: fetch products by id: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p models.Product
		if err := rows.Scan(
			&p.ID, &p.SKU, &p.Name, &p.Description, &p.Category, &p.Gender,
			&p.MetalType, &p.GemstoneType, &p.BasePrice, &p.StockQuantity,
			&p.ImageURL, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("repository: scan product: %w", err)
		}
		out[p.ID] = p
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: iterate products: %w", err)
	}
	return out, nil
}
