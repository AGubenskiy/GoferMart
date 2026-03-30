package handler

import (
	"log/slog"
	"net/http"

	"github.com/AGubenskiy/GoferMart/internal/http/middleware"
	"github.com/AGubenskiy/GoferMart/internal/http/response"
)

type Dependencies struct {
	UserHandler    *UserHandler
	AccountHandler *AccountHandler
	AuthMiddleware func(http.Handler) http.Handler
}

func NewRouter(log *slog.Logger, deps Dependencies) http.Handler {
	mux := http.NewServeMux()

	registerUserRoutes(mux, deps)

	return middleware.Recover(log)(
		middleware.RequestLogger(log)(
			middleware.ResponseCompressor(
				middleware.RequestDecompressor(mux),
			),
		),
	)
}

func registerUserRoutes(mux *http.ServeMux, deps Dependencies) {
	if deps.UserHandler != nil {
		mux.HandleFunc("POST /api/user/register", deps.UserHandler.Register)
		mux.HandleFunc("POST /api/user/login", deps.UserHandler.Login)
	} else {
		mux.HandleFunc("POST /api/user/register", unavailable)
		mux.HandleFunc("POST /api/user/login", unavailable)
	}

	mux.Handle("POST /api/user/orders", protected(deps.AuthMiddleware, handlerOrUnavailable(deps.AccountHandler, (*AccountHandler).UploadOrder)))
	mux.Handle("GET /api/user/orders", protected(deps.AuthMiddleware, handlerOrUnavailable(deps.AccountHandler, (*AccountHandler).ListOrders)))
	mux.Handle("GET /api/user/balance", protected(deps.AuthMiddleware, handlerOrUnavailable(deps.AccountHandler, (*AccountHandler).GetBalance)))
	mux.Handle("POST /api/user/balance/withdraw", protected(deps.AuthMiddleware, handlerOrUnavailable(deps.AccountHandler, (*AccountHandler).Withdraw)))
	mux.Handle("GET /api/user/withdrawals", protected(deps.AuthMiddleware, handlerOrUnavailable(deps.AccountHandler, (*AccountHandler).ListWithdrawals)))
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	response.Status(w, http.StatusNotImplemented)
}

func unavailable(w http.ResponseWriter, _ *http.Request) {
	response.Status(w, http.StatusInternalServerError)
}

func protected(authMiddleware func(http.Handler) http.Handler, next http.Handler) http.Handler {
	if authMiddleware == nil {
		return next
	}

	return authMiddleware(next)
}

func handlerOrUnavailable[T any](value *T, method func(*T, http.ResponseWriter, *http.Request)) http.Handler {
	if value == nil {
		return http.HandlerFunc(unavailable)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method(value, w, r)
	})
}
