package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/AGubenskiy/GoferMart/internal/model"
)

type OrderRepository struct {
	q queryer
}

func (r *OrderRepository) Add(ctx context.Context, userID int64, number string) error {
	const query = `
INSERT INTO orders (number, user_id, status, uploaded_at, updated_at)
VALUES ($1, $2, $3, NOW(), NOW())`

	_, err := r.q.ExecContext(ctx, query, number, userID, model.OrderStatusNew)
	if err == nil {
		return nil
	}

	if !isUniqueViolation(err) {
		return fmt.Errorf("insert order: %w", err)
	}

	ownerID, ownerErr := r.ownerID(ctx, number)
	if ownerErr != nil {
		return ownerErr
	}

	if ownerID == userID {
		return ErrOrderAlreadyUploadedByUser
	}

	return ErrOrderUploadedByAnotherUser
}

func (r *OrderRepository) GetByNumber(ctx context.Context, number string) (model.Order, error) {
	const query = `
SELECT number, user_id, status, accrual, uploaded_at, updated_at
FROM orders
WHERE number = $1`

	order, err := r.scanOne(ctx, query, number)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Order{}, ErrOrderNotFound
		}

		return model.Order{}, fmt.Errorf("get order by number: %w", err)
	}

	return order, nil
}

func (r *OrderRepository) ListByUser(ctx context.Context, userID int64) ([]model.Order, error) {
	const query = `
SELECT number, user_id, status, accrual, uploaded_at, updated_at
FROM orders
WHERE user_id = $1
ORDER BY uploaded_at DESC`

	rows, err := r.q.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list orders by user: %w", err)
	}
	defer rows.Close()

	return scanOrders(rows)
}

func (r *OrderRepository) ListForAccrualSync(ctx context.Context, limit int) ([]model.Order, error) {
	const query = `
SELECT number, user_id, status, accrual, uploaded_at, updated_at
FROM orders
WHERE status IN ($1, $2)
ORDER BY uploaded_at ASC
LIMIT $3`

	rows, err := r.q.QueryContext(ctx, query, model.OrderStatusNew, model.OrderStatusProcessing, limit)
	if err != nil {
		return nil, fmt.Errorf("list orders for accrual sync: %w", err)
	}
	defer rows.Close()

	return scanOrders(rows)
}

func (r *OrderRepository) UpdateStatus(ctx context.Context, number string, status model.OrderStatus, accrual *float64) error {
	const query = `
UPDATE orders
SET status = $2, accrual = $3, updated_at = NOW()
WHERE number = $1`

	result, err := r.q.ExecContext(ctx, query, number, status, accrual)
	if err != nil {
		return fmt.Errorf("update order status: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected rows for order update: %w", err)
	}

	if affected == 0 {
		return ErrOrderNotFound
	}

	return nil
}

func (r *OrderRepository) ownerID(ctx context.Context, number string) (int64, error) {
	var userID int64

	err := r.q.QueryRowContext(ctx, `SELECT user_id FROM orders WHERE number = $1`, number).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrOrderNotFound
		}

		return 0, fmt.Errorf("get order owner: %w", err)
	}

	return userID, nil
}

func (r *OrderRepository) scanOne(ctx context.Context, query string, arg any) (model.Order, error) {
	var (
		order   model.Order
		status  string
		accrual sql.NullFloat64
	)

	err := r.q.QueryRowContext(ctx, query, arg).Scan(
		&order.Number,
		&order.UserID,
		&status,
		&accrual,
		&order.UploadedAt,
		&order.UpdatedAt,
	)
	if err != nil {
		return model.Order{}, err
	}

	order.Status = model.OrderStatus(status)
	if accrual.Valid {
		value := accrual.Float64
		order.Accrual = &value
	}

	return order, nil
}

func scanOrders(rows *sql.Rows) ([]model.Order, error) {
	orders := make([]model.Order, 0)

	for rows.Next() {
		var (
			order   model.Order
			status  string
			accrual sql.NullFloat64
		)

		if err := rows.Scan(
			&order.Number,
			&order.UserID,
			&status,
			&accrual,
			&order.UploadedAt,
			&order.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}

		order.Status = model.OrderStatus(status)
		if accrual.Valid {
			value := accrual.Float64
			order.Accrual = &value
		}

		orders = append(orders, order)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate orders: %w", err)
	}

	return orders, nil
}
