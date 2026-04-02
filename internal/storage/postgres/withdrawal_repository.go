package postgres

import (
	"context"
	"fmt"

	"github.com/AGubenskiy/GoferMart/internal/model"
	"github.com/AGubenskiy/GoferMart/internal/money"
)

type WithdrawalRepository struct {
	q queryer
}

func (r *WithdrawalRepository) Create(ctx context.Context, userID int64, orderNumber string, sum money.Amount) error {
	const query = `
INSERT INTO withdrawals (user_id, order_number, amount, processed_at)
VALUES ($1, $2, $3, NOW())`

	_, err := r.q.ExecContext(ctx, query, userID, orderNumber, sum)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrWithdrawalOrderAlreadyExists
		}

		return fmt.Errorf("create withdrawal: %w", err)
	}

	return nil
}

func (r *WithdrawalRepository) ListByUser(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	const query = `
SELECT id, user_id, order_number, amount::TEXT, processed_at
FROM withdrawals
WHERE user_id = $1
ORDER BY processed_at DESC`

	rows, err := r.q.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list withdrawals by user: %w", err)
	}
	defer rows.Close()

	withdrawals := make([]model.Withdrawal, 0)

	for rows.Next() {
		var withdrawal model.Withdrawal

		if err := rows.Scan(
			&withdrawal.ID,
			&withdrawal.UserID,
			&withdrawal.OrderNumber,
			&withdrawal.Sum,
			&withdrawal.ProcessedAt,
		); err != nil {
			return nil, fmt.Errorf("scan withdrawal: %w", err)
		}

		withdrawals = append(withdrawals, withdrawal)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate withdrawals: %w", err)
	}

	return withdrawals, nil
}

func (r *WithdrawalRepository) GetBalance(ctx context.Context, userID int64) (model.Balance, error) {
	const query = `
SELECT
    COALESCE((
        SELECT SUM(accrual)
        FROM orders
        WHERE user_id = $1 AND status = 'PROCESSED'
    ), 0)::TEXT,
    COALESCE((
        SELECT SUM(amount)
        FROM withdrawals
        WHERE user_id = $1
    ), 0)::TEXT`

	var (
		accrued   money.Amount
		withdrawn money.Amount
	)

	if err := r.q.QueryRowContext(ctx, query, userID).Scan(&accrued, &withdrawn); err != nil {
		return model.Balance{}, fmt.Errorf("get balance: %w", err)
	}

	balance := model.Balance{
		Current:   accrued - withdrawn,
		Withdrawn: withdrawn,
	}

	return balance, nil
}
