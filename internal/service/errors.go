package service

import "errors"

var (
	ErrInvalidInput           = errors.New("invalid input")
	ErrLoginAlreadyTaken      = errors.New("login already taken")
	ErrInvalidCredentials     = errors.New("invalid credentials")
	ErrUnauthorized           = errors.New("unauthorized")
	ErrUnavailable            = errors.New("service unavailable")
	ErrInvalidOrderNumber     = errors.New("invalid order number")
	ErrOrderNumberUnavailable = errors.New("order number unavailable")
	ErrOrderConflict          = errors.New("order conflict")
	ErrInsufficientFunds      = errors.New("insufficient funds")
)
