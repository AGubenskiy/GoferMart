package accrual

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultRetryAttempts = 3
	defaultRetryMinDelay = 100 * time.Millisecond
	defaultRetryMaxDelay = time.Second
)

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type retryingHTTPClient struct {
	next       httpDoer
	attempts   int
	minDelay   time.Duration
	maxDelay   time.Duration
	retrySleep func(context.Context, time.Duration) error
}

func newRetryingHTTPClient(next httpDoer) *retryingHTTPClient {
	return &retryingHTTPClient{
		next:       next,
		attempts:   defaultRetryAttempts,
		minDelay:   defaultRetryMinDelay,
		maxDelay:   defaultRetryMaxDelay,
		retrySleep: sleepContext,
	}
}

func (c *retryingHTTPClient) Do(request *http.Request) (*http.Response, error) {
	attempts := c.retryAttemptsFor(request)

	for attempt := 1; attempt <= attempts; attempt++ {
		attemptRequest, err := cloneRequestForRetry(request, attempt)
		if err != nil {
			return nil, err
		}

		response, err := c.next.Do(attemptRequest)
		if !shouldRetry(request.Context(), response, err) || attempt == attempts {
			return response, err
		}

		if closeErr := discardAndCloseResponse(response); closeErr != nil {
			return nil, closeErr
		}

		if err := c.retrySleep(request.Context(), c.retryDelay(attempt)); err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("retry attempts ended")
}

func (c *retryingHTTPClient) retryAttemptsFor(request *http.Request) int {
	attempts := c.attempts
	if attempts < 1 {
		return 1
	}

	if isRetryableMethod(request.Method) || request.Body == nil || request.GetBody != nil {
		return attempts
	}

	return 1
}

func (c *retryingHTTPClient) retryDelay(attempt int) time.Duration {
	delay := c.minDelay * time.Duration(1<<(attempt-1))
	if delay > c.maxDelay {
		return c.maxDelay
	}

	return delay
}

func cloneRequestForRetry(request *http.Request, attempt int) (*http.Request, error) {
	if attempt == 1 {
		return request, nil
	}

	cloned := request.Clone(request.Context())
	if request.Body == nil {
		return cloned, nil
	}

	if request.GetBody == nil {
		return nil, fmt.Errorf("request body cannot be replayed")
	}

	body, err := request.GetBody()
	if err != nil {
		return nil, fmt.Errorf("recreate request body: %w", err)
	}
	cloned.Body = body

	return cloned, nil
}

func shouldRetry(ctx context.Context, response *http.Response, err error) bool {
	if ctx.Err() != nil {
		return false
	}

	if err != nil {
		return true
	}

	if response == nil {
		return false
	}

	return shouldRetryStatus(response.StatusCode)
}

func shouldRetryStatus(statusCode int) bool {
	if statusCode == http.StatusTooManyRequests {
		return false
	}

	return statusCode >= http.StatusInternalServerError && statusCode != http.StatusNotImplemented
}

func isRetryableMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func discardAndCloseResponse(response *http.Response) error {
	if response == nil || response.Body == nil {
		return nil
	}

	_, copyErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()

	switch {
	case copyErr != nil:
		return fmt.Errorf("discard retryable response body: %w", copyErr)
	case closeErr != nil:
		return fmt.Errorf("close retryable response body: %w", closeErr)
	default:
		return nil
	}
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
