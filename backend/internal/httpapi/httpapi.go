// Package httpapi holds the shared HTTP response and logging conventions for
// the backend. New handlers should use these helpers so error shapes, status
// codes, and request logging stay consistent across modules.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// ErrorBody is the single error response shape for the whole API:
//
//	{ "error": { "code": "not_found", "message": "request not found" } }
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteJSON writes a success payload as JSON.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("write json response", "error", err)
	}
}

// WriteError writes a machine-readable error with a stable code and a
// human-readable message.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorBody{Error: ErrorDetail{Code: code, Message: message}})
}

// NewLogger returns the structured JSON logger used across the backend. Set it
// as the default with slog.SetDefault so package-level slog calls are formatted
// consistently.
func NewLogger() *slog.Logger {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

// statusRecorder captures the response status for request logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// WithRequestLogging logs one structured line per request: method, path,
// status, and duration. It never logs request bodies or query values, which may
// contain sensitive data.
func WithRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
