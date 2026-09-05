package respond

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// JSON writes v as a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// Error writes a JSON error envelope.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

// Fail logs the underlying error (the full cause, so wrapped chains show) for
// diagnosis, then returns a safe public message to the client. Use it instead
// of Error whenever an internal failure is being surfaced to a user, so the
// real cause lands in the logs even though the client only sees `msg`.
func Fail(w http.ResponseWriter, r *http.Request, status int, msg string, err error) {
	slog.Error("request failed",
		"method", r.Method,
		"path", r.URL.Path,
		"status", status,
		"message", msg,
		"error", fmt.Sprintf("%+v", err),
	)
	Error(w, status, msg)
}

// Decode parses the JSON request body into dst, enforcing a sane size limit.
// Unknown fields are rejected so a typo'd setting fails loudly instead of
// being silently dropped.
func Decode(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 2<<20) // 2 MiB
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
