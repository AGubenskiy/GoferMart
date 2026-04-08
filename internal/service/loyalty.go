package service

import (
	"context"
	"errors"
	"strings"

	"github.com/AGubenskiy/GoferMart/internal/model"
	"github.com/AGubenskiy/GoferMart/internal/money"
	"github.com/AGubenskiy/GoferMart/internal/storage/postgres"
	"github.com/AGubenskiy/GoferMart/internal/validate"
)

type LoyaltyService struct {
	store        *postgres.Store
	orders       *postgres.OrderRepository
	orderNumbers *postgres.OrderNumberRepository
	users        *postgres.UserRepository
	withdrawals  *postgres.WithdrawalRepository
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
		store:        store,
		orders:       repos.Orders,
		orderNumbers: repos.OrderNumbers,
		users:        repos.Users,
		withdrawals:  repos.Withdrawals,
	}
}

func (s *LoyaltyService) UploadOrder(ctx context.Context, userID int64, number string) (UploadOrderResult, error) {
	if s == nil {
		return UploadOrderResult{}, ErrUnavailable
	}

	number = strings.TrimSpace(number)
	if !validate.StringLengthAtMost(number, validate.MaxVarcharLength) || !validate.OrderNumber(number) {
		return UploadOrderResult{}, ErrInvalidOrderNumber
	}

	var result UploadOrderResult

	err := s.store.WithTx(ctx, func(repos *postgres.Repositories) error {
		if err := repos.OrderNumbers.Reserve(ctx, userID, number, postgres.OrderNumberKindUpload); err != nil {
			if !errors.Is(err, postgres.ErrOrderNumberAlreadyReserved) {
				return err
			}

			reservation, getErr := repos.OrderNumbers.Get(ctx, number)
			if getErr != nil {
				return getErr
			}

			return applyUploadReservationConflict(reservation, userID, &result)
		}

		if err := repos.Orders.Add(ctx, userID, number); err != nil {
			switch {
			case errors.Is(err, postgres.ErrOrderAlreadyUploadedByUser):
				result = UploadOrderResult{Accepted: false}
				return nil
			case errors.Is(err, postgres.ErrOrderUploadedByAnotherUser):
				return ErrOrderConflict
			default:
				return err
			}
		}

		result = UploadOrderResult{Accepted: true}
		return nil
	})
	if err != nil {
		return UploadOrderResult{}, err
	}

	return result, nil
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

func (s *LoyaltyService) CreateWithdrawal(ctx context.Context, userID int64, orderNumber string, sum money.Amount) error {
	if s == nil {
		return ErrUnavailable
	}

	orderNumber = strings.TrimSpace(orderNumber)
	if !validate.StringLengthAtMost(orderNumber, validate.MaxVarcharLength) || !validate.OrderNumber(orderNumber) {
		return ErrInvalidOrderNumber
	}

	if !sum.IsPositive() {
		return ErrInvalidInput
	}

	return s.store.WithTx(ctx, func(repos *postgres.Repositories) error {
		if err := repos.Users.LockByID(ctx, userID); err != nil {
			return err
		}

		if err := repos.OrderNumbers.Reserve(ctx, userID, orderNumber, postgres.OrderNumberKindWithdrawal); err != nil {
			if errors.Is(err, postgres.ErrOrderNumberAlreadyReserved) {
				return ErrOrderNumberUnavailable
			}

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
				return ErrOrderNumberUnavailable
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

func applyUploadReservationConflict(reservation postgres.OrderNumberReservation, userID int64, result *UploadOrderResult) error {
	if reservation.Kind == postgres.OrderNumberKindUpload && reservation.UserID == userID {
		*result = UploadOrderResult{Accepted: false}
		return nil
	}

	return ErrOrderConflict
}
