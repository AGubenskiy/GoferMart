package handler

import (
	"errors"
	"net/http"

	"github.com/AGubenskiy/GoferMart/internal/auth"
	"github.com/AGubenskiy/GoferMart/internal/http/response"
	"github.com/AGubenskiy/GoferMart/internal/service"
)

type UserHandler struct {
	authService *service.AuthService
	sessions    *auth.SessionManager
}

type credentialsRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func NewUserHandler(authService *service.AuthService, sessions *auth.SessionManager) *UserHandler {
	return &UserHandler{
		authService: authService,
		sessions:    sessions,
	}
}

func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeCredentials(w, r)
	if !ok {
		return
	}

	token, err := h.authService.Register(r.Context(), request.Login, request.Password)
	if err != nil {
		h.writeAuthError(w, err, http.StatusConflict, http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, h.sessions.BuildCookie(token))
	response.Status(w, http.StatusOK)
}

func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeCredentials(w, r)
	if !ok {
		return
	}

	token, err := h.authService.Login(r.Context(), request.Login, request.Password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			response.Status(w, http.StatusBadRequest)
		case errors.Is(err, service.ErrInvalidCredentials):
			response.Status(w, http.StatusUnauthorized)
		case errors.Is(err, service.ErrUnavailable):
			response.Status(w, http.StatusInternalServerError)
		default:
			response.Status(w, http.StatusInternalServerError)
		}
		return
	}

	http.SetCookie(w, h.sessions.BuildCookie(token))
	response.Status(w, http.StatusOK)
}

func decodeCredentials(w http.ResponseWriter, r *http.Request) (credentialsRequest, bool) {
	return decodeJSONRequest[credentialsRequest](w, r)
}

func (h *UserHandler) writeAuthError(w http.ResponseWriter, err error, conflictStatus int, defaultStatus int) {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		response.Status(w, http.StatusBadRequest)
	case errors.Is(err, service.ErrLoginAlreadyTaken):
		response.Status(w, conflictStatus)
	case errors.Is(err, service.ErrUnavailable):
		response.Status(w, http.StatusInternalServerError)
	default:
		response.Status(w, defaultStatus)
	}
}
