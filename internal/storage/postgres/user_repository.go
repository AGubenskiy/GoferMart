package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/AGubenskiy/GoferMart/internal/model"
	"github.com/lib/pq"
)

type UserRepository struct {
	q queryer
}

func (r *UserRepository) Create(ctx context.Context, login, passwordHash string) (model.User, error) {
	const query = `
INSERT INTO users (login, password_hash)
VALUES ($1, $2)
RETURNING id, login, password_hash, created_at`

	var user model.User

	err := r.q.QueryRowContext(ctx, query, login, passwordHash).Scan(
		&user.ID,
		&user.Login,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return model.User{}, ErrDuplicateLogin
		}

		return model.User{}, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (model.User, error) {
	const query = `
SELECT id, login, password_hash, created_at
FROM users
WHERE id = $1`

	return r.getOne(ctx, query, id)
}

func (r *UserRepository) GetByLogin(ctx context.Context, login string) (model.User, error) {
	const query = `
SELECT id, login, password_hash, created_at
FROM users
WHERE login = $1`

	return r.getOne(ctx, query, login)
}

func (r *UserRepository) LockByID(ctx context.Context, id int64) error {
	const query = `SELECT id FROM users WHERE id = $1 FOR UPDATE` //блокировка на строку пользователя

	var lockedID int64
	if err := r.q.QueryRowContext(ctx, query, id).Scan(&lockedID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}

		return fmt.Errorf("lock user by id: %w", err)
	}

	return nil
}

func (r *UserRepository) getOne(ctx context.Context, query string, arg any) (model.User, error) {
	var user model.User

	err := r.q.QueryRowContext(ctx, query, arg).Scan(
		&user.ID,
		&user.Login,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.User{}, ErrUserNotFound
		}

		return model.User{}, fmt.Errorf("query user: %w", err)
	}

	return user, nil
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
