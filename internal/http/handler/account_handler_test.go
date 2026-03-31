package handler

import (
	"net/http"
	"testing"

	"github.com/AGubenskiy/GoferMart/internal/service"
)

func TestWithdrawalErrorStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "invalid order number",
			err:  service.ErrInvalidOrderNumber,
			want: http.StatusUnprocessableEntity,
		},
		{
			name: "reserved number stays in contract",
			err:  service.ErrOrderNumberUnavailable,
			want: http.StatusUnprocessableEntity,
		},
		{
			name: "invalid input",
			err:  service.ErrInvalidInput,
			want: http.StatusBadRequest,
		},
		{
			name: "insufficient funds",
			err:  service.ErrInsufficientFunds,
			want: http.StatusPaymentRequired,
		},
		{
			name: "fallback",
			err:  service.ErrOrderConflict,
			want: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := withdrawalErrorStatus(tt.err); got != tt.want {
				t.Fatalf("withdrawalErrorStatus(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}
