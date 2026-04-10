package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverHandlesPanic(t *testing.T) {
	t.Parallel()

	var logOutput bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logOutput, nil))

	handler := Recover(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/panic", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(logOutput.String(), "panic recovered") {
		t.Fatalf("log output = %q, want panic message", logOutput.String())
	}
	if !strings.Contains(logOutput.String(), "/panic") {
		t.Fatalf("log output = %q, want request path", logOutput.String())
	}
}

func TestRecoverPassesThrough(t *testing.T) {
	t.Parallel()

	handler := Recover(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}
