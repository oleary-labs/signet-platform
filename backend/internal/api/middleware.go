package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/oleary-labs/signet-platform/backend/internal/auth"
	"github.com/oleary-labs/signet-platform/backend/internal/respond"
	"github.com/oleary-labs/signet-platform/backend/internal/store"
)

// requireSession verifies the platform session and attaches the identity.
//
// The token carries only a user id; the role and staff flag are read from the
// database on every request, so revoking someone's access takes effect
// immediately rather than when their token happens to expire.
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			if c, err := r.Cookie(auth.SessionCookieName); err == nil {
				token = c.Value
			}
		}
		if token == "" {
			respond.Error(w, http.StatusUnauthorized, "not signed in")
			return
		}

		claims, err := s.sessions.Verify(token)
		if err != nil {
			respond.Error(w, http.StatusUnauthorized, "session expired — sign in again")
			return
		}

		user, err := s.db.User(r.Context(), claims.UserID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respond.Error(w, http.StatusUnauthorized, "account no longer exists")
				return
			}
			respond.Fail(w, r, http.StatusInternalServerError, "could not load your account", err)
			return
		}

		id := &auth.Identity{
			UserID:      user.ID,
			Subject:     user.Subject,
			SubjectKind: user.SubjectKind,
			DisplayName: user.DisplayName,
			IsStaff:     user.IsStaff,
		}
		if user.Email != nil {
			id.Email = *user.Email
		}
		next.ServeHTTP(w, r.WithContext(auth.WithIdentity(r.Context(), id)))
	})
}

// requireStaff gates the marketplace-curation endpoints.
func (s *Server) requireStaff(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.FromContext(r.Context())
		if !ok || !id.IsStaff {
			respond.Error(w, http.StatusForbidden, "this endpoint is restricted to platform staff")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// identity returns the authenticated caller, or writes a 401 and reports false.
func identity(w http.ResponseWriter, r *http.Request) (*auth.Identity, bool) {
	id, ok := auth.FromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "not signed in")
		return nil, false
	}
	return id, true
}

// urlUUID parses a UUID path parameter, writing a 400 when it is malformed.
func urlUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid "+name)
		return uuid.Nil, false
	}
	return id, true
}

// requireOrg resolves {orgID} and gates on the caller's role.
func (s *Server) requireOrg(w http.ResponseWriter, r *http.Request, required auth.Role) (*auth.Identity, uuid.UUID, bool) {
	id, ok := identity(w, r)
	if !ok {
		return nil, uuid.Nil, false
	}
	orgID, ok := urlUUID(w, r, "orgID")
	if !ok {
		return nil, uuid.Nil, false
	}
	if _, err := s.guard.RequireOrg(r.Context(), id.UserID, orgID, required); err != nil {
		writeAuthzError(w, r, err)
		return nil, uuid.Nil, false
	}
	return id, orgID, true
}

// requireApp resolves {appID} and gates on the caller's role in its org.
func (s *Server) requireApp(w http.ResponseWriter, r *http.Request, required auth.Role) (*auth.Identity, *auth.AppAccess, bool) {
	id, ok := identity(w, r)
	if !ok {
		return nil, nil, false
	}
	appID, ok := urlUUID(w, r, "appID")
	if !ok {
		return nil, nil, false
	}
	access, err := s.guard.RequireApp(r.Context(), id.UserID, appID, required)
	if err != nil {
		writeAuthzError(w, r, err)
		return nil, nil, false
	}
	return id, access, true
}

// writeAuthzError maps guard errors to responses.
//
// A caller who is not a member of an organization gets 404, not 403: telling a
// stranger that an org id exists is itself a disclosure, and they have no way
// to tell the two answers apart anyway.
func writeAuthzError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrForbidden):
		respond.Error(w, http.StatusNotFound, "not found")
	case errors.Is(err, auth.ErrNotFound):
		respond.Error(w, http.StatusNotFound, "not found")
	default:
		respond.Fail(w, r, http.StatusInternalServerError, "authorization check failed", err)
	}
}

// writeStoreError maps store errors to responses for the common cases.
func writeStoreError(w http.ResponseWriter, r *http.Request, err error, action string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		respond.Error(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrConflict):
		respond.Error(w, http.StatusConflict, err.Error())
	default:
		respond.Fail(w, r, http.StatusInternalServerError, action, err)
	}
}
