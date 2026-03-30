package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AGubenskiy/GoferMart/internal/auth"
)

func TestAuthRequiredWithCookie(t *testing.T) {
	t.Parallel()

	manager := auth.NewSessionManager("test-secret", time.Hour)
	token, err := manager.Issue(99)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	handler := AuthRequired(manager)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFromContext(r.Context())
		if !ok {
			t.Fatal("UserIDFromContext() returned ok=false")
		}

		if userID != 99 {
			t.Fatalf("userID = %d, want 99", userID)
		}

		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/user/balance", nil)
	request.AddCookie(manager.BuildCookie(token))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestAuthRequiredWithoutToken(t *testing.T) {
	t.Parallel()

	manager := auth.NewSessionManager("test-secret", time.Hour)
	handler := AuthRequired(manager)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/user/balance", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}
