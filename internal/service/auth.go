package service

import (
	"context"
	"errors"
	"strings"

	"github.com/AGubenskiy/GoferMart/internal/auth"
	"github.com/AGubenskiy/GoferMart/internal/storage/postgres"
	"github.com/AGubenskiy/GoferMart/internal/validate"
)

type AuthService struct {
	users    *postgres.UserRepository
	sessions *auth.SessionManager
}

func NewAuthService(users *postgres.UserRepository, sessions *auth.SessionManager) *AuthService {
	return &AuthService{
		users:    users,
		sessions: sessions,
	}
}

func (s *AuthService) Register(ctx context.Context, login, password string) (string, error) {
	login = strings.TrimSpace(login)
	if login == "" || password == "" || !validate.StringLengthAtMost(login, validate.MaxVarcharLength) {
		return "", ErrInvalidInput
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		if errors.Is(err, auth.ErrWeakPassword) {
			return "", ErrInvalidInput
		}

		return "", err
	}

	user, err := s.users.Create(ctx, login, passwordHash)
	if err != nil {
		if errors.Is(err, postgres.ErrDuplicateLogin) {
			return "", ErrLoginAlreadyTaken
		}

		return "", err
	}

	return s.sessions.Issue(user.ID)
}

func (s *AuthService) Login(ctx context.Context, login, password string) (string, error) {
	login = strings.TrimSpace(login)
	if login == "" || password == "" || len(password) > auth.MaxPasswordBytes || !validate.StringLengthAtMost(login, validate.MaxVarcharLength) { //отсекаю так же слишком длинные пароли
		return "", ErrInvalidInput
	}

	user, err := s.users.GetByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, postgres.ErrUserNotFound) {
			return "", ErrInvalidCredentials
		}

		return "", err
	}

	if err := auth.CheckPassword(user.PasswordHash, password); err != nil {
		return "", ErrInvalidCredentials
	}

	return s.sessions.Issue(user.ID)
}
