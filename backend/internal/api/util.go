package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// chiURLParam reads a path parameter.
func chiURLParam(r *http.Request, name string) string {
	return strings.TrimSpace(chi.URLParam(r, name))
}

// queryInt reads an integer query parameter, falling back to def when it is
// absent or unparsable — a malformed `limit` should not fail a page load.
func queryInt(r *http.Request, name string, def int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

// queryBool reads a boolean query parameter.
func queryBool(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}

// queryDate reads a YYYY-MM-DD query parameter.
func queryDate(r *http.Request, name string, def time.Time) time.Time {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return def
	}
	return t
}

// ptr returns a pointer to v when it is non-empty, and nil otherwise, so a
// PATCH body distinguishes "leave this alone" from "set it to empty".
func ptr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
