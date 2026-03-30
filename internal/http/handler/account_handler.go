package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AGubenskiy/GoferMart/internal/http/middleware"
	"github.com/AGubenskiy/GoferMart/internal/http/response"
	"github.com/AGubenskiy/GoferMart/internal/service"
)

type AccountHandler struct {
	loyalty *service.LoyaltyService
}

type withdrawalRequest struct {
	Order string  `json:"order"`
	Sum   float64 `json:"sum"`
}

type orderResponse struct {
	Number     string   `json:"number"`
	Status     string   `json:"status"`
	Accrual    *float64 `json:"accrual,omitempty"`
	UploadedAt string   `json:"uploaded_at"`
}

type balanceResponse struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

type withdrawalResponse struct {
	Order       string  `json:"order"`
	Sum         float64 `json:"sum"`
	ProcessedAt string  `json:"processed_at"`
}

func NewAccountHandler(loyalty *service.LoyaltyService) *AccountHandler {
	return &AccountHandler{loyalty: loyalty}
}

func (h *AccountHandler) UploadOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Status(w, http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		response.Status(w, http.StatusBadRequest)
		return
	}

	number := strings.TrimSpace(string(payload))
	if number == "" {
		response.Status(w, http.StatusBadRequest)
		return
	}

	result, err := h.loyalty.UploadOrder(r.Context(), userID, number)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidOrderNumber):
			response.Status(w, http.StatusUnprocessableEntity)
		case errors.Is(err, service.ErrOrderConflict):
			response.Status(w, http.StatusConflict)
		case errors.Is(err, service.ErrUnavailable):
			response.Status(w, http.StatusInternalServerError)
		default:
			response.Status(w, http.StatusInternalServerError)
		}
		return
	}

	if result.Accepted {
		response.Status(w, http.StatusAccepted)
		return
	}

	response.Status(w, http.StatusOK)
}

func (h *AccountHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Status(w, http.StatusUnauthorized)
		return
	}

	orders, err := h.loyalty.ListOrders(r.Context(), userID)
	if err != nil {
		response.Status(w, http.StatusInternalServerError)
		return
	}

	if len(orders) == 0 {
		response.Status(w, http.StatusNoContent)
		return
	}

	items := make([]orderResponse, 0, len(orders))
	for _, order := range orders {
		items = append(items, orderResponse{
			Number:     order.Number,
			Status:     string(order.Status),
			Accrual:    order.Accrual,
			UploadedAt: order.UploadedAt.Format(time.RFC3339),
		})
	}

	response.JSON(w, http.StatusOK, items)
}

func (h *AccountHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Status(w, http.StatusUnauthorized)
		return
	}

	balance, err := h.loyalty.GetBalance(r.Context(), userID)
	if err != nil {
		response.Status(w, http.StatusInternalServerError)
		return
	}

	response.JSON(w, http.StatusOK, balanceResponse{
		Current:   balance.Current,
		Withdrawn: balance.Withdrawn,
	})
}

func (h *AccountHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Status(w, http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()

	var request withdrawalRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		response.Status(w, http.StatusBadRequest)
		return
	}

	err := h.loyalty.CreateWithdrawal(r.Context(), userID, request.Order, request.Sum)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidOrderNumber):
			response.Status(w, http.StatusUnprocessableEntity)
		case errors.Is(err, service.ErrInvalidInput):
			response.Status(w, http.StatusBadRequest)
		case errors.Is(err, service.ErrOrderConflict):
			response.Status(w, http.StatusConflict)
		case errors.Is(err, service.ErrInsufficientFunds):
			response.Status(w, http.StatusPaymentRequired)
		case errors.Is(err, service.ErrUnavailable):
			response.Status(w, http.StatusInternalServerError)
		default:
			response.Status(w, http.StatusInternalServerError)
		}
		return
	}

	response.Status(w, http.StatusOK)
}

func (h *AccountHandler) ListWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Status(w, http.StatusUnauthorized)
		return
	}

	withdrawals, err := h.loyalty.ListWithdrawals(r.Context(), userID)
	if err != nil {
		response.Status(w, http.StatusInternalServerError)
		return
	}

	if len(withdrawals) == 0 {
		response.Status(w, http.StatusNoContent)
		return
	}

	items := make([]withdrawalResponse, 0, len(withdrawals))
	for _, withdrawal := range withdrawals {
		items = append(items, withdrawalResponse{
			Order:       withdrawal.OrderNumber,
			Sum:         withdrawal.Sum,
			ProcessedAt: withdrawal.ProcessedAt.Format(time.RFC3339),
		})
	}

	response.JSON(w, http.StatusOK, items)
}
