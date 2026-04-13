package service

import (
	"errors"
	"testing"

	"github.com/AGubenskiy/GoferMart/internal/storage/postgres"
)

func TestApplyUploadReservationConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		reservation postgres.OrderNumberReservation
		userID      int64
		wantResult  UploadOrderResult
		wantErr     error
	}{
		{
			name: "same user repeated upload",
			reservation: postgres.OrderNumberReservation{
				Number: "12345678903",
				UserID: 7,
				Kind:   postgres.OrderNumberKindUpload,
			},
			userID:     7,
			wantResult: UploadOrderResult{Accepted: false},
		},
		{
			name: "other user upload",
			reservation: postgres.OrderNumberReservation{
				Number: "12345678903",
				UserID: 9,
				Kind:   postgres.OrderNumberKindUpload,
			},
			userID:  7,
			wantErr: ErrOrderConflict,
		},
		{
			name: "withdrawal number blocks upload",
			reservation: postgres.OrderNumberReservation{
				Number: "2377225624",
				UserID: 7,
				Kind:   postgres.OrderNumberKindWithdrawal,
			},
			userID:  7,
			wantErr: ErrOrderConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var result UploadOrderResult
			err := applyUploadReservationConflict(tt.reservation, tt.userID, &result)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("applyUploadReservationConflict() error = %v, want %v", err, tt.wantErr)
			}

			if result != tt.wantResult {
				t.Fatalf("applyUploadReservationConflict() result = %+v, want %+v", result, tt.wantResult)
			}
		})
	}
}
