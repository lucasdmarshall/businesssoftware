package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteErrorShape(t *testing.T) {
	recorder := httptest.NewRecorder()
	WriteError(recorder, http.StatusNotFound, "not_found", "request not found")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type %q", got)
	}
	var body ErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response was not valid JSON: %v", err)
	}
	if body.Error.Code != "not_found" || body.Error.Message != "request not found" {
		t.Fatalf("unexpected error body: %+v", body.Error)
	}
}

func TestWriteJSONStatusAndPayload(t *testing.T) {
	recorder := httptest.NewRecorder()
	WriteJSON(recorder, http.StatusCreated, map[string]string{"status": "ok"})

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", recorder.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response was not valid JSON: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestWithRequestLoggingCapturesStatus(t *testing.T) {
	handler := WithRequestLogging(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/x", nil))
	if recorder.Code != http.StatusTeapot {
		t.Fatalf("expected the wrapped status to pass through, got %d", recorder.Code)
	}
}
