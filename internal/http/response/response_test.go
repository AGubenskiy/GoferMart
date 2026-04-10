package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJSONWritesPayload(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	JSON(recorder, http.StatusCreated, map[string]string{"status": "ok"})

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("payload = %v, want status=ok", payload)
	}
}

func TestJSONAllowsNilPayload(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	JSON(recorder, http.StatusNoContent, nil)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("body length = %d, want 0", recorder.Body.Len())
	}
}
