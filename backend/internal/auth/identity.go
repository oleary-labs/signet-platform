package auth

import (
	"context"

	"github.com/google/uuid"
)

// Identity is the authenticated caller, resolved once per request.
type Identity struct {
	UserID      uuid.UUID
	Subject     string
	SubjectKind string
	Email       string
	DisplayName string
	IsStaff     bool
}

type identityKey struct{}

// WithIdentity attaches an identity to the request context.
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// FromContext returns the identity attached by the auth middleware.
func FromContext(ctx context.Context) (*Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(*Identity)
	return id, ok && id != nil
}
