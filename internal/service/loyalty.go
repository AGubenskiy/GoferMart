package service

import (
	"context"
	"errors"
	"strings"

	"github.com/AGubenskiy/GoferMart/internal/model"
	"github.com/AGubenskiy/GoferMart/internal/storage/postgres"
	"github.com/AGubenskiy/GoferMart/internal/validate"
)

type LoyaltyService struct {
	store       *postgres.Store
	orders      *postgres.OrderRepository
	users       *postgres.UserRepository
	withdrawals *postgres.WithdrawalRepository
}

type UploadOrderResult struct {
	Accepted bool
}

func NewLoyaltyService(store *postgres.Store) *LoyaltyService {
	if store == nil {
		return nil
	}

	repos := store.Repositories()

	return &LoyaltyService{
		store:       store,
		orders:      repos.Orders,
		users:       repos.Users,
		withdrawals: repos.Withdrawals,
	}
}

func (s *LoyaltyService) UploadOrder(ctx context.Context, userID int64, number string) (UploadOrderResult, error) {
	if s == nil {
		return UploadOrderResult{}, ErrUnavailable
	}

	number = strings.TrimSpace(number)
	if !validate.OrderNumber(number) {
		return UploadOrderResult{}, ErrInvalidOrderNumber
	}

	err := s.orders.Add(ctx, userID, number)
	if err == nil {
		return UploadOrderResult{Accepted: true}, nil
	}

	switch {
	case errors.Is(err, postgres.ErrOrderAlreadyUploadedByUser):
		return UploadOrderResult{Accepted: false}, nil
	case errors.Is(err, postgres.ErrOrderUploadedByAnotherUser):
		return UploadOrderResult{}, ErrOrderConflict
	default:
		return UploadOrderResult{}, err
	}
}

func (s *LoyaltyService) ListOrders(ctx context.Context, userID int64) ([]model.Order, error) {
	if s == nil {
		return nil, ErrUnavailable
	}

	return s.orders.ListByUser(ctx, userID)
}

func (s *LoyaltyService) GetBalance(ctx context.Context, userID int64) (model.Balance, error) {
	if s == nil {
		return model.Balance{}, ErrUnavailable
	}

	return s.withdrawals.GetBalance(ctx, userID)
}

func (s *LoyaltyService) CreateWithdrawal(ctx context.Context, userID int64, orderNumber string, sum float64) error {
	if s == nil {
		return ErrUnavailable
	}

	orderNumber = strings.TrimSpace(orderNumber)
	if !validate.OrderNumber(orderNumber) {
		return ErrInvalidOrderNumber
	}

	if sum <= 0 {
		return ErrInvalidInput
	}

	return s.store.WithTx(ctx, func(repos *postgres.Repositories) error {
		if err := repos.Users.LockByID(ctx, userID); err != nil {
			return err
		}

		balance, err := repos.Withdrawals.GetBalance(ctx, userID)
		if err != nil {
			return err
		}

		if balance.Current < sum {
			return ErrInsufficientFunds
		}

		if err := repos.Withdrawals.Create(ctx, userID, orderNumber, sum); err != nil {
			if errors.Is(err, postgres.ErrWithdrawalOrderAlreadyExists) {
				return ErrOrderConflict
			}

			return err
		}

		return nil
	})
}

func (s *LoyaltyService) ListWithdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	if s == nil {
		return nil, ErrUnavailable
	}

	return s.withdrawals.ListByUser(ctx, userID)
}
