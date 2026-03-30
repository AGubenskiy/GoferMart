package worker

import (
	"testing"

	"github.com/AGubenskiy/GoferMart/internal/accrual"
	"github.com/AGubenskiy/GoferMart/internal/model"
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
			input:      accrual.Order{Status: accrual.OrderStatusProcessed, Accrual: floatPtr(10)},
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

func floatPtr(value float64) *float64 {
	return &value
}
