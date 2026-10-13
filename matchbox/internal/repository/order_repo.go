package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

type OrderEntity struct {
	ID        string          `json:"id"`
	UserID    string          `json:"user_id"`
	Symbol    string          `json:"symbol"`
	Side      string          `json:"side"`
	Type      string          `json:"type"`
	Price     decimal.Decimal `json:"price"`
	Quantity  decimal.Decimal `json:"quantity"`
	FilledQty decimal.Decimal `json:"filled_qty"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type OrderRepo struct {
	db *pgxpool.Pool
}

func NewOrderRepo(db *pgxpool.Pool) *OrderRepo {
	return &OrderRepo{db: db}
}

func (r *OrderRepo) CreateOrder(ctx context.Context, o *OrderEntity) error {
	query := `
		INSERT INTO orders (id, user_id, symbol, side, type, price, quantity, filled_qty, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := r.db.Exec(ctx, query, o.ID, o.UserID, o.Symbol, o.Side, o.Type, o.Price, o.Quantity, o.FilledQty, o.Status, o.CreatedAt, o.UpdatedAt)
	return err
}

// FetchUserOrders retrieves a user's orders using cursor-based (keyset) pagination.
// Status filter is optional. If passed, it filters by specific statuses (e.g., PENDING).
func (r *OrderRepo) FetchUserOrders(ctx context.Context, userID string, statuses []string, limit int, cursorCreatedAt *time.Time, cursorID *string) ([]OrderEntity, error) {
	// Base query
	query := `
		SELECT id, user_id, symbol, side, type, price, quantity, filled_qty, status, created_at, updated_at
		FROM orders
		WHERE user_id = $1
	`
	
	args := []interface{}{userID}
	argIdx := 2
	
	if len(statuses) > 0 {
		query += fmt.Sprintf(" AND status = ANY($%d)", argIdx)
		args = append(args, statuses)
		argIdx++
	}

	// Cursor condition: (created_at, id) < (cursorCreatedAt, cursorID)
	if cursorCreatedAt != nil && cursorID != nil && !cursorCreatedAt.IsZero() && *cursorID != "" {
		query += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", argIdx, argIdx+1)
		args = append(args, *cursorCreatedAt, *cursorID)
		argIdx += 2
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", argIdx)
	args = append(args, limit)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		if err == pgx.ErrNoRows {
			return []OrderEntity{}, nil
		}
		return nil, fmt.Errorf("failed to fetch orders: %w", err)
	}
	defer rows.Close()

	var orders []OrderEntity
	for rows.Next() {
		var o OrderEntity
		if err := rows.Scan(
			&o.ID, &o.UserID, &o.Symbol, &o.Side, &o.Type, &o.Price, &o.Quantity, &o.FilledQty, &o.Status, &o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan order: %w", err)
		}
		orders = append(orders, o)
	}
	
	return orders, nil
}

// CancelOrder optimizes cancellation by avoiding race conditions via status checks.
func (r *OrderRepo) CancelOrder(ctx context.Context, orderID, userID string) error {
	query := `
		UPDATE orders
		SET status = 'CANCELED', updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND status IN ('PENDING', 'PARTIALLY_FILLED')
	`
	tag, err := r.db.Exec(ctx, query, orderID, userID)
	if err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("order not found, already finished, or belongs to another user")
	}
	return nil
}
