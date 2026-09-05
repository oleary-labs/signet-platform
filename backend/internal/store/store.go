// Package store is the query layer. Handlers never write SQL; every statement
// lives here, so the set of things the platform can do to its data is
// enumerable in one directory.
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a row does not exist. Handlers translate it to
// a 404 rather than leaking a database error.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a uniqueness constraint would be violated —
// a duplicate slug, origin, or issuer.
var ErrConflict = errors.New("already exists")

// Store holds the pool every query runs against.
type Store struct{ pool *pgxpool.Pool }

// New constructs a Store.
func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool exposes the underlying pool for the few callers (health checks,
// background jobs) that need it directly.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// wrap turns pgx's no-rows sentinel into the package's own, so handlers can
// switch on ErrNotFound without importing pgx.
func wrap(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// isUniqueViolation reports whether err is Postgres error 23505.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}

// lower is strings.ToLower, named so call sites in query builders stay short.
func lower(s string) string { return strings.ToLower(s) }

var slugStrip = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify produces a URL-safe slug. It is deliberately lossy and may collide;
// callers that need uniqueness ask the database for a free variant.
func Slugify(name string) string {
	s := slugStrip.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "app"
	}
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	return s
}

// RandomToken returns a URL-safe random string of n bytes of entropy.
func RandomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashToken returns the hex SHA-256 of a token. Invite tokens and app secrets
// are stored only as hashes, so a database copy cannot be used to log in.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
