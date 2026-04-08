package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/AGubenskiy/GoferMart/internal/money"
	"github.com/AGubenskiy/GoferMart/internal/validate"
)

func TestAuthServiceRejectsTooLongLogin(t *testing.T) {
	t.Parallel()

	login := strings.Repeat("a", validate.MaxVarcharLength+1)
	service := &AuthService{}

	if _, err := service.Register(context.Background(), login, "password"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Register() error = %v, want %v", err, ErrInvalidInput)
	}

	if _, err := service.Login(context.Background(), login, "password"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInvalidInput)
	}
}

func TestLoyaltyServiceRejectsTooLongOrderNumber(t *testing.T) {
	t.Parallel()

	number := strings.Repeat("0", validate.MaxVarcharLength+1)
	service := &LoyaltyService{}

	if _, err := service.UploadOrder(context.Background(), 1, number); !errors.Is(err, ErrInvalidOrderNumber) {
		t.Fatalf("UploadOrder() error = %v, want %v", err, ErrInvalidOrderNumber)
	}

	if err := service.CreateWithdrawal(context.Background(), 1, number, money.NewFromCents(100)); !errors.Is(err, ErrInvalidOrderNumber) {
		t.Fatalf("CreateWithdrawal() error = %v, want %v", err, ErrInvalidOrderNumber)
	}
}
