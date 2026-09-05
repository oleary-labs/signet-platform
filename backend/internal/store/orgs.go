package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/oleary-labs/signet-platform/backend/internal/structs"
)

// OrgsForUser lists the organizations a user belongs to, with their role.
func (s *Store) OrgsForUser(ctx context.Context, userID uuid.UUID) ([]structs.Organization, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT o.id, o.name, o.slug, o.logo_url, o.website_url, o.billing_email::text,
		       o.plan, m.role, o.created_at,
		       (SELECT count(*) FROM apps a WHERE a.org_id = o.id AND a.archived_at IS NULL)
		FROM organizations o
		JOIN org_members m ON m.org_id = o.id
		WHERE m.user_id = $1
		ORDER BY o.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.Organization{}
	for rows.Next() {
		var o structs.Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.LogoURL, &o.WebsiteURL,
			&o.BillingEmail, &o.Plan, &o.Role, &o.CreatedAt, &o.AppCount); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Org loads one organization.
func (s *Store) Org(ctx context.Context, orgID uuid.UUID) (*structs.Organization, error) {
	var o structs.Organization
	err := s.pool.QueryRow(ctx, `
		SELECT o.id, o.name, o.slug, o.logo_url, o.website_url, o.billing_email::text,
		       o.plan, o.created_at,
		       (SELECT count(*) FROM apps a WHERE a.org_id = o.id AND a.archived_at IS NULL)
		FROM organizations o WHERE o.id = $1`, orgID,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.LogoURL, &o.WebsiteURL, &o.BillingEmail,
		&o.Plan, &o.CreatedAt, &o.AppCount)
	if err != nil {
		return nil, wrap(err)
	}
	return &o, nil
}

// CreateOrg creates an organization, makes the creator its owner, and opens a
// billing account — all in one transaction, so a half-built org can never be
// left behind for someone to stumble into.
func (s *Store) CreateOrg(ctx context.Context, userID uuid.UUID, name, billingEmail string) (*structs.Organization, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	slug, err := freeSlug(ctx, tx, "organizations", Slugify(name))
	if err != nil {
		return nil, err
	}

	var o structs.Organization
	err = tx.QueryRow(ctx, `
		INSERT INTO organizations (name, slug, billing_email, created_by)
		VALUES ($1, $2, NULLIF($3,'')::citext, $4)
		RETURNING id, name, slug, logo_url, website_url, billing_email::text, plan, created_at`,
		name, slug, billingEmail, userID,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.LogoURL, &o.WebsiteURL, &o.BillingEmail, &o.Plan, &o.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, 'owner')`, o.ID, userID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO billing_accounts (org_id) VALUES ($1)`, o.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	o.Role = "owner"
	return &o, nil
}

// UpdateOrg edits the fields an admin controls.
func (s *Store) UpdateOrg(ctx context.Context, orgID uuid.UUID, name, billingEmail, logoURL, websiteURL string) (*structs.Organization, error) {
	var o structs.Organization
	err := s.pool.QueryRow(ctx, `
		UPDATE organizations SET
			name          = COALESCE(NULLIF($2,''), name),
			billing_email = COALESCE(NULLIF($3,'')::citext, billing_email),
			logo_url      = COALESCE(NULLIF($4,''), logo_url),
			website_url   = COALESCE(NULLIF($5,''), website_url),
			updated_at    = now()
		WHERE id=$1
		RETURNING id, name, slug, logo_url, website_url, billing_email::text, plan, created_at,
		          (SELECT count(*) FROM apps a WHERE a.org_id = organizations.id AND a.archived_at IS NULL)`,
		orgID, name, billingEmail, logoURL, websiteURL,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.LogoURL, &o.WebsiteURL, &o.BillingEmail, &o.Plan, &o.CreatedAt, &o.AppCount)
	if err != nil {
		return nil, wrap(err)
	}
	return &o, nil
}

// Members lists an organization's people.
func (s *Store) Members(ctx context.Context, orgID uuid.UUID) ([]structs.OrgMember, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.subject, u.email::text, u.display_name, u.avatar_url, m.role, m.created_at
		FROM org_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.org_id = $1
		ORDER BY
			CASE m.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 WHEN 'developer' THEN 2 ELSE 3 END,
			m.created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.OrgMember{}
	for rows.Next() {
		var m structs.OrgMember
		if err := rows.Scan(&m.UserID, &m.Subject, &m.Email, &m.DisplayName, &m.AvatarURL, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetMemberRole changes a member's role. The last owner cannot be demoted —
// an org with no owner has no one who can restore access to it.
func (s *Store) SetMemberRole(ctx context.Context, orgID, userID uuid.UUID, role string) error {
	if role != "owner" {
		var owners int
		if err := s.pool.QueryRow(ctx,
			`SELECT count(*) FROM org_members WHERE org_id=$1 AND role='owner'`, orgID).Scan(&owners); err != nil {
			return err
		}
		var current string
		if err := s.pool.QueryRow(ctx,
			`SELECT role FROM org_members WHERE org_id=$1 AND user_id=$2`, orgID, userID).Scan(&current); err != nil {
			return wrap(err)
		}
		if current == "owner" && owners <= 1 {
			return fmt.Errorf("an organization must keep at least one owner")
		}
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE org_members SET role=$3 WHERE org_id=$1 AND user_id=$2`, orgID, userID, role)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveMember removes someone from an organization, refusing to remove the
// last owner for the same reason SetMemberRole refuses to demote them.
func (s *Store) RemoveMember(ctx context.Context, orgID, userID uuid.UUID) error {
	var role string
	if err := s.pool.QueryRow(ctx,
		`SELECT role FROM org_members WHERE org_id=$1 AND user_id=$2`, orgID, userID).Scan(&role); err != nil {
		return wrap(err)
	}
	if role == "owner" {
		var owners int
		if err := s.pool.QueryRow(ctx,
			`SELECT count(*) FROM org_members WHERE org_id=$1 AND role='owner'`, orgID).Scan(&owners); err != nil {
			return err
		}
		if owners <= 1 {
			return fmt.Errorf("an organization must keep at least one owner")
		}
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM org_members WHERE org_id=$1 AND user_id=$2`, orgID, userID)
	return err
}

// CreateInvite issues an invitation and returns the raw token exactly once.
func (s *Store) CreateInvite(ctx context.Context, orgID, invitedBy uuid.UUID, email, role string, ttl time.Duration) (*structs.OrgInvite, error) {
	token, err := RandomToken(24)
	if err != nil {
		return nil, err
	}
	var inv structs.OrgInvite
	err = s.pool.QueryRow(ctx, `
		INSERT INTO org_invites (org_id, email, role, token_hash, invited_by, expires_at)
		VALUES ($1, $2::citext, $3, $4, $5, now() + $6::interval)
		RETURNING id, email::text, role, expires_at, created_at`,
		orgID, email, role, HashToken(token), invitedBy, ttl.String(),
	).Scan(&inv.ID, &inv.Email, &inv.Role, &inv.ExpiresAt, &inv.CreatedAt)
	if err != nil {
		return nil, err
	}
	inv.Token = token
	return &inv, nil
}

// Invites lists an org's outstanding invitations.
func (s *Store) Invites(ctx context.Context, orgID uuid.UUID) ([]structs.OrgInvite, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, email::text, role, expires_at, created_at, accepted_at
		FROM org_invites
		WHERE org_id=$1 AND revoked_at IS NULL
		ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.OrgInvite{}
	for rows.Next() {
		var inv structs.OrgInvite
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Role, &inv.ExpiresAt, &inv.CreatedAt, &inv.AcceptedAt); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// RevokeInvite cancels an outstanding invitation.
func (s *Store) RevokeInvite(ctx context.Context, orgID, inviteID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE org_invites SET revoked_at=now()
		 WHERE id=$1 AND org_id=$2 AND accepted_at IS NULL AND revoked_at IS NULL`, inviteID, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AcceptInvite redeems a token and joins the user to the organization.
//
// The invite is claimed with a conditional UPDATE inside the transaction, so
// two people racing the same link cannot both join, and the membership insert
// is idempotent for someone who is already a member.
func (s *Store) AcceptInvite(ctx context.Context, userID uuid.UUID, token string) (*structs.Organization, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	var orgID uuid.UUID
	var role string
	err = tx.QueryRow(ctx, `
		UPDATE org_invites
		SET accepted_at = now(), accepted_by = $2
		WHERE token_hash = $1
		  AND accepted_at IS NULL
		  AND revoked_at IS NULL
		  AND expires_at > now()
		RETURNING org_id, role`, HashToken(token), userID).Scan(&orgID, &role)
	if err != nil {
		return nil, wrap(err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO org_members (org_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (org_id, user_id) DO NOTHING`, orgID, userID, role); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.Org(ctx, orgID)
}

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, so slug allocation
// can run inside the caller's transaction.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// freeSlug finds an unused slug for a table, appending -2, -3 … on collision.
// The bound is there so a pathological case fails loudly rather than spinning.
func freeSlug(ctx context.Context, q querier, table, base string) (string, error) {
	for i := 1; i <= 50; i++ {
		candidate := base
		if i > 1 {
			candidate = fmt.Sprintf("%s-%d", base, i)
		}
		var exists bool
		query := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE slug=$1)`, table)
		if err := q.QueryRow(ctx, query, candidate).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not find a free slug for %q", base)
}
