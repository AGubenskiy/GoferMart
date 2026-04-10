package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLoggerLogsHandledRequest(t *testing.T) {
	t.Parallel()

	var logOutput bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logOutput, nil))

	handler := RequestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/user/orders", nil)
	request.RemoteAddr = "127.0.0.1:8080"

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusAccepted)
	}

	logged := logOutput.String()
	for _, want := range []string{"request handled", "POST", "/api/user/orders", "127.0.0.1:8080"} {
		if !strings.Contains(logged, want) {
			t.Fatalf("log output = %q, want substring %q", logged, want)
		}
	}
}
