package model

import (
	"time"

	"github.com/AGubenskiy/GoferMart/internal/money"
)

type OrderStatus string

const (
	OrderStatusNew        OrderStatus = "NEW"
	OrderStatusProcessing OrderStatus = "PROCESSING"
	OrderStatusInvalid    OrderStatus = "INVALID"
	OrderStatusProcessed  OrderStatus = "PROCESSED"
)

type Order struct {
	Number     string
	UserID     int64
	Status     OrderStatus
	Accrual    *money.Amount
	UploadedAt time.Time
	UpdatedAt  time.Time
}
