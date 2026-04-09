package handler

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/AGubenskiy/GoferMart/internal/http/middleware"
	"github.com/AGubenskiy/GoferMart/internal/http/response"
)

func authenticatedUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return 0, false
	}

	return userID, true
}

func closeRequestBody(r *http.Request) {
	if r.Body == nil {
		return
	}

	_ = r.Body.Close()
}

func decodeJSONRequest[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	defer closeRequestBody(r)

	var request T
	if !matchesContentType(r, "application/json") {
		w.WriteHeader(http.StatusBadRequest)
		return request, false
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return request, false
	}

	return request, true
}

func readTextRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	defer closeRequestBody(r)

	if !matchesContentType(r, "text/plain") {
		w.WriteHeader(http.StatusBadRequest)
		return "", false
	}

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return "", false
	}

	text := strings.TrimSpace(string(payload))
	if text == "" {
		w.WriteHeader(http.StatusBadRequest)
		return "", false
	}

	return text, true
}

func matchesContentType(r *http.Request, expected string) bool {
	value := strings.TrimSpace(r.Header.Get("Content-Type"))
	if value == "" {
		return true
	}

	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}

	return strings.EqualFold(mediaType, expected)
}

func writeUserCollection[T any, R any](
	w http.ResponseWriter,
	r *http.Request,
	list func(context.Context, int64) ([]T, error),
	mapItem func(T) R,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	items, err := list(r.Context(), userID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if len(items) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	payload := make([]R, 0, len(items))
	for _, item := range items {
		payload = append(payload, mapItem(item))
	}

	response.JSON(w, http.StatusOK, payload)
}
