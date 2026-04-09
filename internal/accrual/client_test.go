package accrual

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AGubenskiy/GoferMart/internal/money"
)

func TestClientGetOrderProcessed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/orders/12345678903" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"order":"12345678903","status":"PROCESSED","accrual":500.5}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	order, err := client.GetOrder(context.Background(), "12345678903")
	if err != nil {
		t.Fatalf("GetOrder() error = %v", err)
	}

	if order.Status != OrderStatusProcessed {
		t.Fatalf("GetOrder() status = %s, want %s", order.Status, OrderStatusProcessed)
	}

	if order.Accrual == nil || *order.Accrual != money.NewFromCents(50050) {
		t.Fatalf("GetOrder() accrual = %v, want 500.5", order.Accrual)
	}
}

func TestClientGetOrderNotRegistered(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.GetOrder(context.Background(), "12345678903")
	if !errors.Is(err, ErrOrderNotRegistered) {
		t.Fatalf("GetOrder() error = %v, want %v", err, ErrOrderNotRegistered)
	}
}

func TestClientGetOrderRateLimit(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.GetOrder(context.Background(), "12345678903")
	var rateLimitErr *RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("GetOrder() error = %v, want RateLimitError", err)
	}

	if rateLimitErr.RetryAfter != time.Minute {
		t.Fatalf("RetryAfter = %s, want %s", rateLimitErr.RetryAfter, time.Minute)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("accrual calls = %d, want 1", got)
	}
}

func TestClientGetOrderRetriesTemporaryServerErrors(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"order":"12345678903","status":"PROCESSED","accrual":500.5}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	useImmediateRetries(client)

	order, err := client.GetOrder(context.Background(), "12345678903")
	if err != nil {
		t.Fatalf("GetOrder() error = %v", err)
	}

	if order.Status != OrderStatusProcessed {
		t.Fatalf("GetOrder() status = %s, want %s", order.Status, OrderStatusProcessed)
	}

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("accrual calls = %d, want 3", got)
	}
}

func useImmediateRetries(client *Client) {
	retryingClient, ok := client.httpClient.(*retryingHTTPClient)
	if !ok {
		return
	}

	retryingClient.retrySleep = func(context.Context, time.Duration) error {
		return nil
	}
}
