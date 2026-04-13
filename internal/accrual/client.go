package accrual

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/AGubenskiy/GoferMart/internal/money"
)

var ErrOrderNotRegistered = errors.New("order not registered")

type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("accrual rate limit exceeded, retry after %s", e.RetryAfter)
}

type OrderStatus string

const (
	OrderStatusRegistered OrderStatus = "REGISTERED"
	OrderStatusProcessing OrderStatus = "PROCESSING"
	OrderStatusInvalid    OrderStatus = "INVALID"
	OrderStatusProcessed  OrderStatus = "PROCESSED"
)

type Order struct {
	Number  string
	Status  OrderStatus
	Accrual *money.Amount
}

type Client struct {
	baseURL    *url.URL
	httpClient httpDoer
}

type orderResponse struct {
	Order   string        `json:"order"`
	Status  string        `json:"status"`
	Accrual *money.Amount `json:"accrual"`
}

func NewClient(address string) (*Client, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("accrual address is empty")
	}

	baseURL, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("parse accrual address: %w", err)
	}

	if baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("accrual address must include scheme and host")
	}

	return &Client{
		baseURL: baseURL,
		httpClient: newRetryingHTTPClient(&http.Client{
			Timeout: 5 * time.Second,
		}),
	}, nil
}

func (c *Client) GetOrder(ctx context.Context, number string) (Order, error) {
	requestURL := *c.baseURL
	requestURL.Path = path.Join(c.baseURL.Path, "/api/orders", number)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return Order{}, fmt.Errorf("build accrual request: %w", err)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return Order{}, fmt.Errorf("perform accrual request: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	switch response.StatusCode {
	case http.StatusOK:
		var payload orderResponse
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			return Order{}, fmt.Errorf("decode accrual response: %w", err)
		}

		return Order{
			Number:  payload.Order,
			Status:  OrderStatus(payload.Status),
			Accrual: payload.Accrual,
		}, nil

	case http.StatusNoContent:
		return Order{}, ErrOrderNotRegistered

	case http.StatusTooManyRequests:
		return Order{}, &RateLimitError{RetryAfter: retryAfter(response.Header.Get("Retry-After"))}

	default:
		return Order{}, fmt.Errorf("unexpected accrual status: %d", response.StatusCode)
	}
}

func retryAfter(headerValue string) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(headerValue))
	if err != nil || seconds <= 0 {
		return time.Minute
	}

	return time.Duration(seconds) * time.Second
}
