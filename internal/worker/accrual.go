package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/AGubenskiy/GoferMart/internal/accrual"
	"github.com/AGubenskiy/GoferMart/internal/model"
	"github.com/AGubenskiy/GoferMart/internal/storage/postgres"
)

type AccrualWorker struct {
	log       *slog.Logger
	client    *accrual.Client
	orders    *postgres.OrderRepository
	interval  time.Duration
	batchSize int
}

func NewAccrualWorker(log *slog.Logger, client *accrual.Client, orders *postgres.OrderRepository) *AccrualWorker {
	if log == nil || client == nil || orders == nil {
		return nil
	}

	return &AccrualWorker{
		log:       log,
		client:    client,
		orders:    orders,
		interval:  2 * time.Second,
		batchSize: 20,
	}
}

func (w *AccrualWorker) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("accrual worker stopped")
			return

		case <-timer.C:
			delay := w.sync(ctx)
			timer.Reset(delay)
		}
	}
}

func (w *AccrualWorker) sync(ctx context.Context) time.Duration {
	orders, err := w.orders.ListForAccrualSync(ctx, w.batchSize)
	if err != nil {
		w.log.Error("failed to list orders for accrual sync", "error", err)
		return w.interval
	}

	if len(orders) == 0 {
		return w.interval
	}

	for _, order := range orders {
		delay, err := w.syncOrder(ctx, order)
		if err != nil {
			var rateLimitErr *accrual.RateLimitError
			if errors.As(err, &rateLimitErr) {
				w.log.Warn("accrual rate limit reached", "retry_after", rateLimitErr.RetryAfter.String())
				return rateLimitErr.RetryAfter
			}

			w.log.Error("failed to sync order with accrual system", "order", order.Number, "error", err)
			continue
		}

		if delay > 0 {
			return delay
		}
	}

	return w.interval
}

func (w *AccrualWorker) syncOrder(ctx context.Context, order model.Order) (time.Duration, error) {
	result, err := w.client.GetOrder(ctx, order.Number)
	if err != nil {
		if errors.Is(err, accrual.ErrOrderNotRegistered) {
			return 0, nil
		}

		var rateLimitErr *accrual.RateLimitError
		if errors.As(err, &rateLimitErr) {
			return rateLimitErr.RetryAfter, rateLimitErr
		}

		return 0, err
	}

	status, accrualValue := mapAccrualOrder(result)
	if err := w.orders.UpdateStatus(ctx, order.Number, status, accrualValue); err != nil {
		return 0, err
	}

	return 0, nil
}

func mapAccrualOrder(order accrual.Order) (model.OrderStatus, *float64) {
	switch order.Status {
	case accrual.OrderStatusInvalid:
		return model.OrderStatusInvalid, nil
	case accrual.OrderStatusProcessed:
		return model.OrderStatusProcessed, order.Accrual
	case accrual.OrderStatusRegistered, accrual.OrderStatusProcessing:
		return model.OrderStatusProcessing, nil
	default:
		return model.OrderStatusProcessing, nil
	}
}
