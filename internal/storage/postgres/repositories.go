package postgres

import (
	"context"
	"database/sql"
)

type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Repositories struct {
	Users        *UserRepository
	Orders       *OrderRepository
	OrderNumbers *OrderNumberRepository
	Withdrawals  *WithdrawalRepository
}

func newRepositories(q queryer) *Repositories {
	return &Repositories{
		Users:        &UserRepository{q: q},
		Orders:       &OrderRepository{q: q},
		OrderNumbers: &OrderNumberRepository{q: q},
		Withdrawals:  &WithdrawalRepository{q: q},
	}
}
