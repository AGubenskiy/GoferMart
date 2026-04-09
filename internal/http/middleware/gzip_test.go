package middleware

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponseCompressorLogsCloseError(t *testing.T) {
	t.Parallel()

	var logOutput bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logOutput, nil))
	response := &failingResponseWriter{header: make(http.Header)}

	handler := ResponseCompressor(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("response"))
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
	request.Header.Set("Accept-Encoding", "gzip")

	handler.ServeHTTP(response, request)

	if !strings.Contains(logOutput.String(), "close gzip response writer") {
		t.Fatalf("log output = %q, want close error message", logOutput.String())
	}
}

type failingResponseWriter struct {
	header http.Header
}

func (w *failingResponseWriter) Header() http.Header {
	return w.header
}

func (w *failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func (w *failingResponseWriter) WriteHeader(int) {}
