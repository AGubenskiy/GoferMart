package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/AGubenskiy/GoferMart/internal/accrual"
	"github.com/AGubenskiy/GoferMart/internal/model"
	"github.com/AGubenskiy/GoferMart/internal/money"
)

func TestMapAccrualOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      accrual.Order
		wantStatus model.OrderStatus
		wantNil    bool
	}{
		{
			name:       "registered becomes processing",
			input:      accrual.Order{Status: accrual.OrderStatusRegistered},
			wantStatus: model.OrderStatusProcessing,
			wantNil:    true,
		},
		{
			name:       "invalid stays invalid",
			input:      accrual.Order{Status: accrual.OrderStatusInvalid},
			wantStatus: model.OrderStatusInvalid,
			wantNil:    true,
		},
		{
			name:       "processed keeps accrual",
			input:      accrual.Order{Status: accrual.OrderStatusProcessed, Accrual: amountPtr(money.NewFromCents(1000))},
			wantStatus: model.OrderStatusProcessed,
			wantNil:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, accrualValue := mapAccrualOrder(tt.input)
			if status != tt.wantStatus {
				t.Fatalf("status = %s, want %s", status, tt.wantStatus)
			}

			if (accrualValue == nil) != tt.wantNil {
				t.Fatalf("accrual nil = %v, want %v", accrualValue == nil, tt.wantNil)
			}
		})
	}
}

func TestNewAccrualWorkerRejectsNilDependencies(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := accrualClientFunc(func(ctx context.Context, number string) (accrual.Order, error) {
		return accrual.Order{}, nil
	})

	if worker, err := NewAccrualWorker(nil, client, &fakeOrderRepository{}); worker != nil || !errors.Is(err, errMissingDependencies) {
		t.Fatalf("NewAccrualWorker() with nil logger = (%v, %v), want nil, %v", worker, err, errMissingDependencies)
	}

	var nilClient *fakeAccrualClient
	if worker, err := NewAccrualWorker(log, nilClient, &fakeOrderRepository{}); worker != nil || !errors.Is(err, errMissingDependencies) {
		t.Fatalf("NewAccrualWorker() with typed nil client = (%v, %v), want nil, %v", worker, err, errMissingDependencies)
	}

	var nilOrders *fakeOrderRepository
	if worker, err := NewAccrualWorker(log, client, nilOrders); worker != nil || !errors.Is(err, errMissingDependencies) {
		t.Fatalf("NewAccrualWorker() with typed nil repository = (%v, %v), want nil, %v", worker, err, errMissingDependencies)
	}

	if worker, err := NewAccrualWorker(log, client, &fakeOrderRepository{}); err != nil || worker == nil {
		t.Fatalf("NewAccrualWorker() valid dependencies = (%v, %v), want worker, nil", worker, err)
	}
}

func TestSyncOrdersRunsRequestsConcurrently(t *testing.T) {
	t.Parallel()

	started := make(chan string, 2)
	release := make(chan struct{})

	client := accrualClientFunc(func(ctx context.Context, number string) (accrual.Order, error) {
		select {
		case started <- number:
		case <-ctx.Done():
			return accrual.Order{}, ctx.Err()
		}

		select {
		case <-release:
			return accrual.Order{Status: accrual.OrderStatusProcessing}, nil
		case <-ctx.Done():
			return accrual.Order{}, ctx.Err()
		}
	})

	worker := newTestAccrualWorker(client, 2)
	done := make(chan time.Duration, 1)

	go func() {
		done <- worker.syncOrders(context.Background(), []model.Order{
			{Number: "1"},
			{Number: "2"},
		})
	}()

	startedNumbers := map[string]bool{
		waitStartedOrder(t, started): true,
		waitStartedOrder(t, started): true,
	}
	if !startedNumbers["1"] || !startedNumbers["2"] {
		t.Fatalf("started orders = %v, want 1 and 2", startedNumbers)
	}

	select {
	case delay := <-done:
		t.Fatalf("syncOrders() finished before parallel requests were released; delay=%s", delay)
	default:
	}

	close(release)

	if delay := waitSyncOrdersDone(t, done); delay != 0 {
		t.Fatalf("syncOrders() delay = %s, want 0", delay)
	}
}

func TestSyncOrdersStopsBatchOnRateLimit(t *testing.T) {
	t.Parallel()

	const retryAfter = 3 * time.Second

	started := make(chan string, 4)

	client := accrualClientFunc(func(ctx context.Context, number string) (accrual.Order, error) {
		started <- number

		if number == "1" {
			return accrual.Order{}, &accrual.RateLimitError{RetryAfter: retryAfter}
		}

		<-ctx.Done()
		return accrual.Order{}, ctx.Err()
	})

	worker := newTestAccrualWorker(client, 2)
	delay := worker.syncOrders(context.Background(), []model.Order{
		{Number: "1"},
		{Number: "2"},
		{Number: "3"},
		{Number: "4"},
	})

	if delay != retryAfter {
		t.Fatalf("syncOrders() delay = %s, want %s", delay, retryAfter)
	}

	startedNumbers := drainStartedOrders(started)
	if startedNumbers["3"] || startedNumbers["4"] {
		t.Fatalf("orders started after rate limit = %v, want no 3/4", startedNumbers)
	}
}

func amountPtr(value money.Amount) *money.Amount {
	return &value
}

type accrualClientFunc func(ctx context.Context, number string) (accrual.Order, error)

func (f accrualClientFunc) GetOrder(ctx context.Context, number string) (accrual.Order, error) {
	return f(ctx, number)
}

type fakeOrderRepository struct{}

type fakeAccrualClient struct{}

func (c *fakeAccrualClient) GetOrder(context.Context, string) (accrual.Order, error) {
	return accrual.Order{}, nil
}

func (r *fakeOrderRepository) ListForAccrualSync(_ context.Context, _ int) ([]model.Order, error) {
	return nil, nil
}

func (r *fakeOrderRepository) UpdateStatus(
	_ context.Context,
	_ string,
	_ model.OrderStatus,
	_ *money.Amount,
) error {
	return nil
}

func newTestAccrualWorker(client accrualClient, workers int) *AccrualWorker {
	return &AccrualWorker{
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		client:    client,
		orders:    &fakeOrderRepository{},
		interval:  time.Second,
		batchSize: 20,
		workers:   workers,
	}
}

func waitStartedOrder(t *testing.T, started <-chan string) string {
	t.Helper()

	select {
	case number := <-started:
		return number
	case <-time.After(time.Second):
		t.Fatal("order was not started before timeout")
		return ""
	}
}

func waitSyncOrdersDone(t *testing.T, done <-chan time.Duration) time.Duration {
	t.Helper()

	select {
	case delay := <-done:
		return delay
	case <-time.After(time.Second):
		t.Fatal("syncOrders() did not finish before timeout")
		return 0
	}
}

func drainStartedOrders(started <-chan string) map[string]bool {
	orders := make(map[string]bool)

	for {
		select {
		case number := <-started:
			orders[number] = true
		default:
			return orders
		}
	}
}
