package store

import (
	"context"
	"time"
)

// Cross-tenant reads for the staff overview.
//
// Every other query in this package is scoped to one organization, because the
// console is multi-tenant and a developer must never see another's apps. These
// deliberately are not, which is why they live in their own file and why every
// route reaching them sits behind requireStaff.
//
// What they exist for: sponsorship is open, so the platform pays to deploy
// groups for people nobody vetted. "Watch for abuse and react" is only a
// strategy if someone can see. Before this, the first signal would have been
// the paymaster balance dropping.

// StaffTotals is the whole deployment in one row.
type StaffTotals struct {
	Users           int `json:"users"`
	SignetUsers     int `json:"signet_users"`
	WalletUsers     int `json:"wallet_users"`
	Organizations   int `json:"organizations"`
	Apps            int `json:"apps"`
	DeployedGroups  int `json:"deployed_groups"`
	SponsoredGroups int `json:"sponsored_groups"`
	NewUsers24h     int `json:"new_users_24h"`
	NewGroups24h    int `json:"new_groups_24h"`
}

func (s *Store) StaffTotals(ctx context.Context) (*StaffTotals, error) {
	var t StaffTotals
	err := s.pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM users),
		  (SELECT count(*) FROM users WHERE subject_kind = 'signet'),
		  (SELECT count(*) FROM users WHERE subject_kind = 'siwe'),
		  (SELECT count(*) FROM organizations),
		  (SELECT count(*) FROM apps WHERE archived_at IS NULL),
		  (SELECT count(*) FROM apps WHERE group_address IS NOT NULL),
		  (SELECT count(*) FROM apps WHERE group_address IS NOT NULL AND environment = 'development'),
		  (SELECT count(*) FROM users WHERE created_at > now() - interval '24 hours'),
		  (SELECT count(*) FROM apps WHERE deployed_at > now() - interval '24 hours')`,
	).Scan(&t.Users, &t.SignetUsers, &t.WalletUsers, &t.Organizations, &t.Apps,
		&t.DeployedGroups, &t.SponsoredGroups, &t.NewUsers24h, &t.NewGroups24h)
	if err != nil {
		return nil, wrap(err)
	}
	return &t, nil
}

// StaffUser is one account with what it has cost and created.
type StaffUser struct {
	Subject     string  `json:"subject"`
	SubjectKind string  `json:"subject_kind"`
	DisplayName string  `json:"display_name"`
	Email       *string `json:"email"`
	IsStaff     bool    `json:"is_staff"`
	Orgs        int     `json:"orgs"`
	Apps        int     `json:"apps"`
	// SponsoredGroups is what this person has drawn from the paymaster, on the
	// same definition the deploy route enforces its ceiling against.
	SponsoredGroups int        `json:"sponsored_groups"`
	CreatedAt       time.Time  `json:"created_at"`
	LastSeenAt      *time.Time `json:"last_seen_at"`
}

// StaffUsers lists accounts, heaviest sponsorship users first — the ordering
// that answers the question the page exists for.
func (s *Store) StaffUsers(ctx context.Context, limit int) ([]StaffUser, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT u.subject, u.subject_kind, u.display_name, u.email::text, u.is_staff,
		       (SELECT count(*) FROM org_members m WHERE m.user_id = u.id),
		       (SELECT count(*) FROM apps a WHERE a.created_by = u.id AND a.archived_at IS NULL),
		       (SELECT count(*) FROM apps a WHERE a.created_by = u.id
		          AND a.group_address IS NOT NULL AND a.environment = 'development'),
		       u.created_at, u.last_seen_at
		FROM users u
		ORDER BY 8 DESC, u.created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()

	out := []StaffUser{}
	for rows.Next() {
		var u StaffUser
		if err := rows.Scan(&u.Subject, &u.SubjectKind, &u.DisplayName, &u.Email, &u.IsStaff,
			&u.Orgs, &u.Apps, &u.SponsoredGroups, &u.CreatedAt, &u.LastSeenAt); err != nil {
			return nil, wrap(err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// StaffApp is one app across every tenant, with who owns it.
type StaffApp struct {
	Name         string     `json:"name"`
	OrgName      string     `json:"org_name"`
	CreatorSub   *string    `json:"creator_subject"`
	Environment  string     `json:"environment"`
	Status       string     `json:"status"`
	ChainID      int64      `json:"chain_id"`
	GroupAddress *string    `json:"group_address"`
	Threshold    *int       `json:"threshold"`
	NodeCount    *int       `json:"node_count"`
	Sponsored    bool       `json:"sponsored"`
	DeployedAt   *time.Time `json:"deployed_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (s *Store) StaffApps(ctx context.Context, limit int) ([]StaffApp, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT a.name, o.name, u.subject, a.environment, a.status, a.chain_id,
		       a.group_address, a.threshold, a.node_count,
		       (a.group_address IS NOT NULL AND a.environment = 'development'),
		       a.deployed_at, a.created_at
		FROM apps a
		JOIN organizations o ON o.id = a.org_id
		LEFT JOIN users u ON u.id = a.created_by
		ORDER BY a.created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()

	out := []StaffApp{}
	for rows.Next() {
		var a StaffApp
		if err := rows.Scan(&a.Name, &a.OrgName, &a.CreatorSub, &a.Environment, &a.Status,
			&a.ChainID, &a.GroupAddress, &a.Threshold, &a.NodeCount, &a.Sponsored,
			&a.DeployedAt, &a.CreatedAt); err != nil {
			return nil, wrap(err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
