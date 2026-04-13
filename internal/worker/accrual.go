package worker

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/AGubenskiy/GoferMart/internal/accrual"
	"github.com/AGubenskiy/GoferMart/internal/model"
	"github.com/AGubenskiy/GoferMart/internal/money"
)

const defaultAccrualWorkerCount = 4

type accrualClient interface {
	GetOrder(ctx context.Context, number string) (accrual.Order, error)
}

type orderRepository interface {
	ListForAccrualSync(ctx context.Context, limit int) ([]model.Order, error)
	UpdateStatus(ctx context.Context, number string, status model.OrderStatus, accrual *money.Amount) error
}

type AccrualWorker struct {
	log       *slog.Logger
	client    accrualClient
	orders    orderRepository
	interval  time.Duration
	batchSize int
	workers   int
}

type syncOrderResult struct {
	order model.Order
	err   error
}

var errMissingDependencies = errors.New("missing required dependencies for worker")

func NewAccrualWorker(log *slog.Logger, client accrualClient, orders orderRepository) (*AccrualWorker, error) {
	if log == nil || isNilDependency(client) || isNilDependency(orders) {
		return nil, errMissingDependencies
	}

	return &AccrualWorker{
		log:       log,
		client:    client,
		orders:    orders,
		interval:  2 * time.Second,
		batchSize: 20,
		workers:   defaultAccrualWorkerCount,
	}, nil
}

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
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
		if ctx.Err() != nil {
			return w.interval
		}

		w.log.Error("failed to list orders for accrual sync", "error", err)
		return w.interval
	}

	if len(orders) == 0 {
		return w.interval
	}

	if delay := w.syncOrders(ctx, orders); delay > 0 {
		return delay
	}

	return w.interval
}

func (w *AccrualWorker) syncOrders(ctx context.Context, orders []model.Order) time.Duration {
	workerCount := w.workerCount(len(orders))
	if workerCount == 0 {
		return 0
	}

	batchCtx, cancelBatch := context.WithCancel(ctx)
	defer cancelBatch()

	jobs := make(chan model.Order)
	results := make(chan syncOrderResult, workerCount)

	var workersDone sync.WaitGroup
	workersDone.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		go func() {
			defer workersDone.Done()
			w.runSyncWorker(batchCtx, cancelBatch, jobs, results)
		}()
	}

	go func() {
		defer close(results)
		workersDone.Wait()
	}()

	go enqueueAccrualJobs(batchCtx, jobs, orders)

	var retryAfter time.Duration
	for result := range results {
		if result.err == nil {
			continue
		}

		var rateLimitErr *accrual.RateLimitError
		if errors.As(result.err, &rateLimitErr) {
			if retryAfter == 0 {
				w.log.Warn("accrual rate limit reached", "retry_after", rateLimitErr.RetryAfter.String())
			}
			retryAfter = maxDuration(retryAfter, rateLimitErr.RetryAfter)
			continue
		}

		if isContextStopped(batchCtx, result.err) {
			continue
		}

		w.log.Error("failed to sync order with accrual system", "order", result.order.Number, "error", result.err)
	}

	return retryAfter
}

func (w *AccrualWorker) workerCount(orderCount int) int {
	if orderCount <= 0 {
		return 0
	}

	workerCount := w.workers
	if workerCount < 1 {
		workerCount = defaultAccrualWorkerCount
	}

	if workerCount > orderCount {
		return orderCount
	}

	return workerCount
}

func (w *AccrualWorker) runSyncWorker(
	ctx context.Context,
	cancelBatch context.CancelFunc,
	jobs <-chan model.Order,
	results chan<- syncOrderResult,
) {
	for {
		select {
		case <-ctx.Done():
			return

		case order, ok := <-jobs:
			if !ok {
				return
			}

			err := w.syncOrder(ctx, order)
			if isAccrualRateLimit(err) {
				cancelBatch()
			}

			results <- syncOrderResult{order: order, err: err}
		}
	}
}

func enqueueAccrualJobs(ctx context.Context, jobs chan<- model.Order, orders []model.Order) {
	defer close(jobs)

	for _, order := range orders {
		select {
		case <-ctx.Done():
			return
		case jobs <- order:
		}
	}
}

func (w *AccrualWorker) syncOrder(ctx context.Context, order model.Order) error {
	result, err := w.client.GetOrder(ctx, order.Number)
	if err != nil {
		if errors.Is(err, accrual.ErrOrderNotRegistered) {
			return nil
		}

		var rateLimitErr *accrual.RateLimitError
		if errors.As(err, &rateLimitErr) {
			return rateLimitErr
		}

		return err
	}

	status, accrualValue := mapAccrualOrder(result)
	if err := w.orders.UpdateStatus(ctx, order.Number, status, accrualValue); err != nil {
		return err
	}

	return nil
}

func mapAccrualOrder(order accrual.Order) (model.OrderStatus, *money.Amount) {
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

func isAccrualRateLimit(err error) bool {
	var rateLimitErr *accrual.RateLimitError
	return errors.As(err, &rateLimitErr)
}

func isContextStopped(ctx context.Context, err error) bool {
	return ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
}

func maxDuration(left, right time.Duration) time.Duration {
	if right > left {
		return right
	}

	return left
}
