package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type contentTypePayload struct {
	Value string `json:"value"`
}

func TestDecodeJSONRequestContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		wantOK      bool
	}{
		{name: "json", contentType: "application/json", wantOK: true},
		{name: "json with charset", contentType: "application/json; charset=utf-8", wantOK: true},
		{name: "empty header", contentType: "", wantOK: true},
		{name: "wrong media type", contentType: "text/plain", wantOK: false},
		{name: "invalid header", contentType: "application/json; charset", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(`{"value":"ok"}`))
			if tt.contentType != "" {
				request.Header.Set("Content-Type", tt.contentType)
			}

			recorder := httptest.NewRecorder()
			payload, ok := decodeJSONRequest[contentTypePayload](recorder, request)

			if ok != tt.wantOK {
				t.Fatalf("decodeJSONRequest() ok = %v, want %v", ok, tt.wantOK)
			}

			if !tt.wantOK {
				if recorder.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
				}
				return
			}

			if payload.Value != "ok" {
				t.Fatalf("payload.Value = %q, want %q", payload.Value, "ok")
			}
		})
	}
}

func TestReadTextRequestContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		wantOK      bool
	}{
		{name: "text", contentType: "text/plain", wantOK: true},
		{name: "text with charset", contentType: "text/plain; charset=utf-8", wantOK: true},
		{name: "empty header", contentType: "", wantOK: true},
		{name: "wrong media type", contentType: "application/json", wantOK: false},
		{name: "invalid header", contentType: "text/plain; charset", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodPost, "/api/user/orders", strings.NewReader("12345678903"))
			if tt.contentType != "" {
				request.Header.Set("Content-Type", tt.contentType)
			}

			recorder := httptest.NewRecorder()
			payload, ok := readTextRequest(recorder, request)

			if ok != tt.wantOK {
				t.Fatalf("readTextRequest() ok = %v, want %v", ok, tt.wantOK)
			}

			if !tt.wantOK {
				if recorder.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
				}
				return
			}

			if payload != "12345678903" {
				t.Fatalf("payload = %q, want %q", payload, "12345678903")
			}
		})
	}
}
