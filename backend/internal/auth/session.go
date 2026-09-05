package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// SessionCookieName is the cookie the browser console uses. API clients may
// instead send the same token as a bearer credential.
const SessionCookieName = "signet_platform_session"

// ErrSession is returned for any unusable session token. Callers treat every
// variety the same way — re-authenticate — so the reason stays internal.
var ErrSession = errors.New("invalid session")

// SessionClaims are the platform's own session claims. They carry only what
// middleware needs to resolve an identity without a database round-trip on
// every request; roles are always re-read from the database, never trusted
// from the token, so a permission change takes effect immediately.
type SessionClaims struct {
	UserID  uuid.UUID `json:"uid"`
	Subject string    `json:"sub"`
	Method  string    `json:"amr"`
	jwt.RegisteredClaims
}

// SessionIssuer mints and verifies platform session tokens.
type SessionIssuer struct {
	secret []byte
	ttl    time.Duration
}

// NewSessionIssuer builds an issuer. A short TTL is deliberate: the console
// silently refreshes, and a stolen token expires quickly.
func NewSessionIssuer(secret string, ttlSeconds int) *SessionIssuer {
	if ttlSeconds <= 0 {
		ttlSeconds = 60 * 60 * 12
	}
	return &SessionIssuer{secret: []byte(secret), ttl: time.Duration(ttlSeconds) * time.Second}
}

// TTL is the lifetime of the tokens this issuer mints.
func (s *SessionIssuer) TTL() time.Duration { return s.ttl }

// Issue mints a session token for a user.
func (s *SessionIssuer) Issue(userID uuid.UUID, subject, method string) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(s.ttl)
	claims := SessionClaims{
		UserID:  userID,
		Subject: subject,
		Method:  method,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "signet-platform",
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        uuid.NewString(),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign session: %w", err)
	}
	return signed, exp, nil
}

// Verify parses and validates a session token.
func (s *SessionIssuer) Verify(token string) (*SessionClaims, error) {
	claims := &SessionClaims{}
	// Pinning the algorithm is what stops an attacker swapping HS256 for
	// "none" (or for RS256 with our secret as the public key).
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		return s.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer("signet-platform"))
	if err != nil || !parsed.Valid {
		return nil, ErrSession
	}
	if claims.UserID == uuid.Nil {
		return nil, ErrSession
	}
	return claims, nil
}
