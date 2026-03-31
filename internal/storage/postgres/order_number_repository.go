package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type OrderNumberKind string

const (
	OrderNumberKindUpload     OrderNumberKind = "UPLOAD"
	OrderNumberKindWithdrawal OrderNumberKind = "WITHDRAWAL"
)

type OrderNumberReservation struct {
	Number string
	UserID int64
	Kind   OrderNumberKind
}

type OrderNumberRepository struct {
	q queryer
}

func (r *OrderNumberRepository) Reserve(ctx context.Context, userID int64, number string, kind OrderNumberKind) error {
	//noinspection SqlNoDataSourceInspection
	const query = `
INSERT INTO order_numbers (number, user_id, kind, created_at)
VALUES ($1, $2, $3, NOW())`

	_, err := r.q.ExecContext(ctx, query, number, userID, kind)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrOrderNumberAlreadyReserved
		}

		return fmt.Errorf("reserve order number: %w", err)
	}

	return nil
}

func (r *OrderNumberRepository) Get(ctx context.Context, number string) (OrderNumberReservation, error) {
	//noinspection SqlNoDataSourceInspection
	const query = `
SELECT number, user_id, kind
FROM order_numbers
WHERE number = $1`

	var reservation OrderNumberReservation

	err := r.q.QueryRowContext(ctx, query, number).Scan(
		&reservation.Number,
		&reservation.UserID,
		&reservation.Kind,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OrderNumberReservation{}, ErrOrderNumberReservationMissed
		}

		return OrderNumberReservation{}, fmt.Errorf("get order number reservation: %w", err)
	}

	return reservation, nil
}
