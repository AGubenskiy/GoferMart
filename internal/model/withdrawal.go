package model

import (
	"time"

	"github.com/AGubenskiy/GoferMart/internal/money"
)

type Withdrawal struct {
	ID          int64
	UserID      int64
	OrderNumber string
	Sum         money.Amount
	ProcessedAt time.Time
}
