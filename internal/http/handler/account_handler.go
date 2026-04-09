package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/AGubenskiy/GoferMart/internal/http/response"
	"github.com/AGubenskiy/GoferMart/internal/model"
	"github.com/AGubenskiy/GoferMart/internal/money"
	"github.com/AGubenskiy/GoferMart/internal/service"
)

type AccountHandler struct {
	loyalty *service.LoyaltyService
}

type withdrawalRequest struct {
	Order string       `json:"order"`
	Sum   money.Amount `json:"sum"`
}

type orderResponse struct {
	Number     string        `json:"number"`
	Status     string        `json:"status"`
	Accrual    *money.Amount `json:"accrual,omitempty"`
	UploadedAt string        `json:"uploaded_at"`
}

type balanceResponse struct {
	Current   money.Amount `json:"current"`
	Withdrawn money.Amount `json:"withdrawn"`
}

type withdrawalResponse struct {
	Order       string       `json:"order"`
	Sum         money.Amount `json:"sum"`
	ProcessedAt string       `json:"processed_at"`
}

func NewAccountHandler(loyalty *service.LoyaltyService) *AccountHandler {
	return &AccountHandler{loyalty: loyalty}
}

func (h *AccountHandler) UploadOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	number, ok := readTextRequest(w, r)
	if !ok {
		return
	}

	result, err := h.loyalty.UploadOrder(r.Context(), userID, number)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidOrderNumber):
			w.WriteHeader(http.StatusUnprocessableEntity)
		case errors.Is(err, service.ErrOrderConflict):
			w.WriteHeader(http.StatusConflict)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	if result.Accepted {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *AccountHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	writeUserCollection(w, r, h.loyalty.ListOrders, mapOrderResponse)
}

func (h *AccountHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	balance, err := h.loyalty.GetBalance(r.Context(), userID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	response.JSON(w, http.StatusOK, balanceResponse{
		Current:   balance.Current,
		Withdrawn: balance.Withdrawn,
	})
}

func (h *AccountHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	request, ok := decodeJSONRequest[withdrawalRequest](w, r)
	if !ok {
		return
	}

	err := h.loyalty.CreateWithdrawal(r.Context(), userID, request.Order, request.Sum)
	if err != nil {
		w.WriteHeader(withdrawalErrorStatus(err))
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *AccountHandler) ListWithdrawals(w http.ResponseWriter, r *http.Request) {
	writeUserCollection(w, r, h.loyalty.ListWithdrawals, mapWithdrawalResponse)
}

func mapOrderResponse(order model.Order) orderResponse {
	return orderResponse{
		Number:     order.Number,
		Status:     string(order.Status),
		Accrual:    order.Accrual,
		UploadedAt: order.UploadedAt.Format(time.RFC3339),
	}
}

func mapWithdrawalResponse(withdrawal model.Withdrawal) withdrawalResponse {
	return withdrawalResponse{
		Order:       withdrawal.OrderNumber,
		Sum:         withdrawal.Sum,
		ProcessedAt: withdrawal.ProcessedAt.Format(time.RFC3339),
	}
}

func withdrawalErrorStatus(err error) int {
	switch {
	case errors.Is(err, service.ErrInvalidOrderNumber), errors.Is(err, service.ErrOrderNumberUnavailable):
		return http.StatusUnprocessableEntity
	case errors.Is(err, service.ErrInvalidInput):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrInsufficientFunds):
		return http.StatusPaymentRequired
	default:
		return http.StatusInternalServerError
	}
}
