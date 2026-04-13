package model

import "github.com/AGubenskiy/GoferMart/internal/money"

type Balance struct {
	Current   money.Amount
	Withdrawn money.Amount
}
