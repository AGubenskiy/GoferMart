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
	Users       *UserRepository
	Orders      *OrderRepository
	Withdrawals *WithdrawalRepository
}

func newRepositories(q queryer) *Repositories {
	return &Repositories{
		Users:       &UserRepository{q: q},
		Orders:      &OrderRepository{q: q},
		Withdrawals: &WithdrawalRepository{q: q},
	}
}
