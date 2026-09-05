package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Role is a member's role within an organization. Ordering matters: Require
// compares rank, so a new role must be inserted at the right rank rather than
// appended.
type Role string

const (
	RoleViewer    Role = "viewer"
	RoleDeveloper Role = "developer"
	RoleAdmin     Role = "admin"
	RoleOwner     Role = "owner"
)

var roleRank = map[Role]int{
	RoleViewer:    1,
	RoleDeveloper: 2,
	RoleAdmin:     3,
	RoleOwner:     4,
}

// AtLeast reports whether r satisfies the required role.
func (r Role) AtLeast(required Role) bool {
	return roleRank[r] >= roleRank[required]
}

// Valid reports whether r is a recognized role.
func (r Role) Valid() bool { _, ok := roleRank[r]; return ok }

var (
	// ErrForbidden means the caller is known but not permitted.
	ErrForbidden = errors.New("forbidden")
	// ErrNotFound means the target org or app does not exist (or is archived).
	ErrNotFound = errors.New("not found")
)

// Guard answers "may this user do this to this org/app?" against the database.
// Roles are never cached in the session token, so revoking access takes effect
// on the caller's very next request.
type Guard struct{ pool *pgxpool.Pool }

// NewGuard constructs a Guard.
func NewGuard(pool *pgxpool.Pool) *Guard { return &Guard{pool: pool} }

// OrgRole returns the caller's role in an organization.
func (g *Guard) OrgRole(ctx context.Context, userID, orgID uuid.UUID) (Role, error) {
	var role Role
	err := g.pool.QueryRow(ctx,
		`SELECT role FROM org_members WHERE org_id=$1 AND user_id=$2`, orgID, userID,
	).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	if err != nil {
		return "", fmt.Errorf("read org role: %w", err)
	}
	return role, nil
}

// RequireOrg gates access to an organization at the given role.
func (g *Guard) RequireOrg(ctx context.Context, userID, orgID uuid.UUID, required Role) (Role, error) {
	role, err := g.OrgRole(ctx, userID, orgID)
	if err != nil {
		return "", err
	}
	if !role.AtLeast(required) {
		return role, ErrForbidden
	}
	return role, nil
}

// AppAccess is the resolved context for an app-scoped request.
type AppAccess struct {
	AppID uuid.UUID
	OrgID uuid.UUID
	Role  Role
}

// RequireApp resolves an app to its owning org and gates on the caller's role
// there. Apps have no membership of their own — access is inherited from the
// organization, which keeps one list of people to reason about.
func (g *Guard) RequireApp(ctx context.Context, userID, appID uuid.UUID, required Role) (*AppAccess, error) {
	var orgID uuid.UUID
	err := g.pool.QueryRow(ctx,
		`SELECT org_id FROM apps WHERE id=$1 AND archived_at IS NULL`, appID,
	).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("resolve app org: %w", err)
	}
	role, err := g.RequireOrg(ctx, userID, orgID, required)
	if err != nil {
		return nil, err
	}
	return &AppAccess{AppID: appID, OrgID: orgID, Role: role}, nil
}
