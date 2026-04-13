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
	return s.orders.ListByUser(ctx, userID)
}

func (s *LoyaltyService) GetBalance(ctx context.Context, userID int64) (model.Balance, error) {
	return s.withdrawals.GetBalance(ctx, userID)
}

func (s *LoyaltyService) CreateWithdrawal(ctx context.Context, userID int64, orderNumber string, sum money.Amount) error {
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
		} // блокировка SELECT ... FOR UPDATE;

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
		//дал ответ в другой ветке продублирую тут
		////
		//Должно работать. Дефолтный уровень изоляции PostgreSQL Read Committed.
		//Списание идет внутри одной транзакции и используется блокировка SELECT ... FOR UPDATE;
		//Значит несколько параллельных списаний одного и того же пользователя не идут одновременно, последующее ждёт, пока первое завершится.
		//Похоже на пессимистичную блокировку для конкретного пользователя.
		//if err := repos.Users.LockByID(ctx, userID); err != nil {
		//	return err
		//}
		//
		//func (r *UserRepository) LockByID(ctx context.Context, id int64) error {
		//	const query = SELECT id FROM users WHERE id = $1 FOR UPDATE
		//
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
	return s.withdrawals.ListByUser(ctx, userID)
}

func applyUploadReservationConflict(reservation postgres.OrderNumberReservation, userID int64, result *UploadOrderResult) error {
	if reservation.Kind == postgres.OrderNumberKindUpload && reservation.UserID == userID {
		*result = UploadOrderResult{Accepted: false}
		return nil
	}

	return ErrOrderConflict
}
