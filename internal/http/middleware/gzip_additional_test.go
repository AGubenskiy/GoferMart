package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestDecompressorInflatesBody(t *testing.T) {
	t.Parallel()

	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte("12345678903")); err != nil {
		t.Fatalf("gzip write error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip close error = %v", err)
	}

	var gotBody string
	handler := RequestDecompressor(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		gotBody = string(payload)
		if encoding := r.Header.Get("Content-Encoding"); encoding != "" {
			t.Fatalf("Content-Encoding = %q, want empty", encoding)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/user/orders", bytes.NewReader(compressed.Bytes()))
	request.Header.Set("Content-Encoding", "gzip")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if gotBody != "12345678903" {
		t.Fatalf("body = %q, want %q", gotBody, "12345678903")
	}
}

func TestRequestDecompressorRejectsInvalidGzip(t *testing.T) {
	t.Parallel()

	handler := RequestDecompressor(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/user/orders", bytes.NewBufferString("not-gzip"))
	request.Header.Set("Content-Encoding", "gzip")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestResponseCompressorCompressesBody(t *testing.T) {
	t.Parallel()

	handler := ResponseCompressor(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("compressed response"))
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
	request.Header.Set("Accept-Encoding", "gzip")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := recorder.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}

	reader, err := gzip.NewReader(bytes.NewReader(recorder.Body.Bytes()))
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}
	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(body) != "compressed response" {
		t.Fatalf("body = %q, want %q", string(body), "compressed response")
	}
}

func TestGzipResponseWriterCloseWithoutBodyWritesStatus(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	writer := newGzipResponseWriter(recorder)

	writer.WriteHeader(http.StatusCreated)
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
}

func TestGzipResponseWriterWriteHeaderWithoutBodyStatus(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	writer := newGzipResponseWriter(recorder)

	writer.WriteHeader(http.StatusNoContent)
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestShouldWriteBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status int
		want   bool
	}{
		{status: http.StatusContinue, want: false},
		{status: http.StatusOK, want: true},
		{status: http.StatusCreated, want: true},
		{status: http.StatusNoContent, want: false},
		{status: http.StatusNotModified, want: false},
	}

	for _, tt := range tests {
		if got := shouldWriteBody(tt.status); got != tt.want {
			t.Fatalf("shouldWriteBody(%d) = %v, want %v", tt.status, got, tt.want)
		}
	}
}
