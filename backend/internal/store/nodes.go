package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/oleary-labs/signet-platform/backend/internal/structs"
)

const operatorColumns = `
	n.address, n.name, n.slug, n.description, n.website_url, n.logo_url, n.api_url,
	n.region, n.jurisdiction, n.category, n.verified, n.is_open, n.registered_at,
	n.operator_address, n.group_count, n.synced_at`

func scanOperator(row interface{ Scan(...any) error }) (*structs.NodeOperator, error) {
	var n structs.NodeOperator
	err := row.Scan(&n.Address, &n.Name, &n.Slug, &n.Description, &n.WebsiteURL, &n.LogoURL,
		&n.APIURL, &n.Region, &n.Jurisdiction, &n.Category, &n.Verified, &n.IsOpen,
		&n.RegisteredAt, &n.OperatorAddress, &n.GroupCount, &n.SyncedAt)
	if err != nil {
		return nil, wrap(err)
	}
	return &n, nil
}

// NodeOperatorFilter narrows the marketplace listing.
type NodeOperatorFilter struct {
	Category   string
	Region     string
	OnlyOpen   bool
	OnlyOnline bool
	Search     string
}

// NodeOperators lists the marketplace directory with recent health attached.
//
// Health comes from a lateral join over the last 24 hours of probes rather
// than a stored flag, so a node that stopped responding an hour ago shows as
// offline without anything having to write to the operator row.
func (s *Store) NodeOperators(ctx context.Context, f NodeOperatorFilter) ([]structs.NodeOperator, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+operatorColumns+`,
		       h.ok, h.latency_ms, h.peer_count, h.error, h.observed_at, u.uptime_pct
		FROM node_operators n
		LEFT JOIN LATERAL (
			SELECT ok, latency_ms, peer_count, error, observed_at
			FROM node_health_samples
			WHERE node_address = n.address
			ORDER BY observed_at DESC
			LIMIT 1
		) h ON true
		LEFT JOIN LATERAL (
			SELECT (100.0 * count(*) FILTER (WHERE ok) / NULLIF(count(*), 0)) AS uptime_pct
			FROM node_health_samples
			WHERE node_address = n.address AND observed_at > now() - interval '24 hours'
		) u ON true
		WHERE ($1 = '' OR n.category = $1)
		  AND ($2 = '' OR n.region = $2)
		  AND (NOT $3 OR n.is_open IS TRUE)
		  AND (NOT $4 OR h.ok IS TRUE)
		  AND ($5 = '' OR n.name ILIKE '%' || $5 || '%' OR n.description ILIKE '%' || $5 || '%')
		ORDER BY n.verified DESC, h.ok DESC NULLS LAST, n.name`,
		f.Category, f.Region, f.OnlyOpen, f.OnlyOnline, f.Search)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.NodeOperator{}
	for rows.Next() {
		var n structs.NodeOperator
		var h structs.NodeHealth
		var ok *bool
		if err := rows.Scan(&n.Address, &n.Name, &n.Slug, &n.Description, &n.WebsiteURL, &n.LogoURL,
			&n.APIURL, &n.Region, &n.Jurisdiction, &n.Category, &n.Verified, &n.IsOpen,
			&n.RegisteredAt, &n.OperatorAddress, &n.GroupCount, &n.SyncedAt,
			&ok, &h.LatencyMS, &h.PeerCount, &h.LastError, &h.ObservedAt, &h.UptimePct24h); err != nil {
			return nil, err
		}
		// A node with no probe yet reports no health at all rather than
		// "offline" — never having been asked is not the same as being down.
		if ok != nil {
			h.Online = *ok
			n.Health = &h
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// NodeOperator loads a single operator by address or slug.
func (s *Store) NodeOperator(ctx context.Context, addressOrSlug string) (*structs.NodeOperator, error) {
	n, err := scanOperator(s.pool.QueryRow(ctx,
		`SELECT `+operatorColumns+` FROM node_operators n
		 WHERE lower(n.address)=lower($1) OR n.slug=$1`, addressOrSlug))
	if err != nil {
		return nil, err
	}
	health, err := s.NodeHealth(ctx, n.Address)
	if err != nil {
		return nil, err
	}
	n.Health = health
	return n, nil
}

// NodeHealth summarises the last probe and 24-hour uptime for one node.
func (s *Store) NodeHealth(ctx context.Context, address string) (*structs.NodeHealth, error) {
	var h structs.NodeHealth
	var ok *bool
	err := s.pool.QueryRow(ctx, `
		SELECT h.ok, h.latency_ms, h.peer_count, h.error, h.observed_at,
		       (SELECT 100.0 * count(*) FILTER (WHERE ok) / NULLIF(count(*),0)
		        FROM node_health_samples
		        WHERE node_address=$1 AND observed_at > now() - interval '24 hours')
		FROM node_health_samples h
		WHERE h.node_address=$1
		ORDER BY h.observed_at DESC
		LIMIT 1`, address,
	).Scan(&ok, &h.LatencyMS, &h.PeerCount, &h.LastError, &h.ObservedAt, &h.UptimePct24h)
	if err != nil {
		if err := wrap(err); err == ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	if ok != nil {
		h.Online = *ok
	}
	return &h, nil
}

// UpsertNodeOperatorInput is the curated metadata for a marketplace listing.
type UpsertNodeOperatorInput struct {
	Address      string
	Name         string
	Description  string
	WebsiteURL   string
	LogoURL      string
	APIURL       string
	Region       string
	Jurisdiction string
	Category     string
	ContactEmail string
	Verified     *bool
}

// UpsertNodeOperator creates or edits a marketplace listing. Only platform
// staff reach this; the chain-derived fields are refreshed separately by
// SyncNodeRegistry so a staff edit never overwrites on-chain truth.
func (s *Store) UpsertNodeOperator(ctx context.Context, in UpsertNodeOperatorInput) (*structs.NodeOperator, error) {
	slug := Slugify(in.Name)
	if in.Category == "" {
		in.Category = "independent"
	}
	return scanOperator(s.pool.QueryRow(ctx, `
		INSERT INTO node_operators
			(address, name, slug, description, website_url, logo_url, api_url,
			 region, jurisdiction, category, contact_email, verified)
		VALUES (lower($1), $2, $3, $4, NULLIF($5,''), NULLIF($6,''), NULLIF($7,''),
		        $8, $9, $10, NULLIF($11,'')::citext, COALESCE($12, false))
		ON CONFLICT (address) DO UPDATE SET
			name         = EXCLUDED.name,
			description  = EXCLUDED.description,
			website_url  = COALESCE(EXCLUDED.website_url, node_operators.website_url),
			logo_url     = COALESCE(EXCLUDED.logo_url, node_operators.logo_url),
			api_url      = COALESCE(EXCLUDED.api_url, node_operators.api_url),
			region       = EXCLUDED.region,
			jurisdiction = EXCLUDED.jurisdiction,
			category     = EXCLUDED.category,
			contact_email = COALESCE(EXCLUDED.contact_email, node_operators.contact_email),
			verified     = COALESCE($12, node_operators.verified),
			updated_at   = now()
		RETURNING `+strippedOperatorColumns,
		in.Address, in.Name, slug, in.Description, in.WebsiteURL, in.LogoURL, in.APIURL,
		in.Region, in.Jurisdiction, in.Category, in.ContactEmail, in.Verified))
}

const strippedOperatorColumns = `
	address, name, slug, description, website_url, logo_url, api_url,
	region, jurisdiction, category, verified, is_open, registered_at,
	operator_address, group_count, synced_at`

// SyncNodeRegistry refreshes the on-chain half of an operator row. An address
// the chain knows but the directory does not gets a placeholder listing, so a
// newly registered node appears in the marketplace immediately rather than
// waiting for someone to write its marketing copy.
func (s *Store) SyncNodeRegistry(ctx context.Context, address string, isOpen bool, registeredAt time.Time, operator string, groupCount int) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO node_operators (address, name, slug, description, is_open, registered_at, operator_address, group_count, synced_at)
		VALUES (lower($1), $2, $3, '', $4, $5, lower(NULLIF($6,'')), $7, now())
		ON CONFLICT (address) DO UPDATE SET
			is_open          = EXCLUDED.is_open,
			registered_at    = EXCLUDED.registered_at,
			operator_address = EXCLUDED.operator_address,
			group_count      = EXCLUDED.group_count,
			synced_at        = now()`,
		address, shortAddress(address), "node-"+shortSlug(address), isOpen, registeredAt, operator, groupCount)
	return err
}

// RecordHealthSample stores one probe result.
func (s *Store) RecordHealthSample(ctx context.Context, address string, ok bool, latencyMS, peers int, probeErr string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO node_health_samples (node_address, ok, latency_ms, peer_count, error)
		VALUES (lower($1), $2, NULLIF($3,0), NULLIF($4,0), NULLIF($5,''))`,
		address, ok, latencyMS, peers, probeErr)
	return err
}

// PruneHealthSamples drops probe history past the retention window. Seven days
// is enough for the 24-hour uptime figure and for a human to look back at an
// incident, without the table growing without bound.
func (s *Store) PruneHealthSamples(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM node_health_samples WHERE observed_at < now() - interval '7 days'`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ProbeTargets lists nodes with an API URL to probe.
func (s *Store) ProbeTargets(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT address, api_url FROM node_operators WHERE api_url IS NOT NULL AND api_url <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var addr, url string
		if err := rows.Scan(&addr, &url); err != nil {
			return nil, err
		}
		out[addr] = url
	}
	return out, rows.Err()
}

// ─────────────────────────── Group membership ───────────────────────────

// GroupNodes lists an app's group membership with operator branding attached.
func (s *Store) GroupNodes(ctx context.Context, appID uuid.UUID) ([]structs.GroupNode, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT an.node_address, an.status, an.execute_after, an.removal_initiator, an.joined_at,
		       n.address, n.name, n.slug, n.description, n.website_url, n.logo_url, n.api_url,
		       n.region, n.jurisdiction, n.category, n.verified, n.is_open, n.registered_at,
		       n.operator_address, n.group_count, n.synced_at,
		       h.ok, h.latency_ms, h.observed_at
		FROM app_nodes an
		LEFT JOIN node_operators n ON n.address = an.node_address
		LEFT JOIN LATERAL (
			SELECT ok, latency_ms, observed_at FROM node_health_samples
			WHERE node_address = an.node_address ORDER BY observed_at DESC LIMIT 1
		) h ON true
		WHERE an.app_id = $1
		ORDER BY
			CASE an.status WHEN 'active' THEN 0 WHEN 'pending' THEN 1 ELSE 2 END,
			an.node_address`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.GroupNode{}
	for rows.Next() {
		var g structs.GroupNode
		var op structs.NodeOperator
		var opAddress *string
		var healthOK *bool
		var latency *int
		var observed *time.Time
		if err := rows.Scan(&g.Address, &g.Status, &g.ExecuteAfter, &g.Initiator, &g.JoinedAt,
			&opAddress, &op.Name, &op.Slug, &op.Description, &op.WebsiteURL, &op.LogoURL, &op.APIURL,
			&op.Region, &op.Jurisdiction, &op.Category, &op.Verified, &op.IsOpen, &op.RegisteredAt,
			&op.OperatorAddress, &op.GroupCount, &op.SyncedAt,
			&healthOK, &latency, &observed); err != nil {
			return nil, err
		}
		if opAddress != nil {
			op.Address = *opAddress
			if healthOK != nil {
				op.Health = &structs.NodeHealth{Online: *healthOK, LatencyMS: latency, ObservedAt: observed}
			}
			g.Operator = &op
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GroupNodeState is one row of the membership snapshot the indexer writes.
type GroupNodeState struct {
	Address      string
	Status       string
	ExecuteAfter *time.Time
	Initiator    string
}

// ReplaceGroupNodes swaps an app's cached membership for a fresh snapshot.
// Replacing wholesale (rather than merging) is what lets a node that left the
// group disappear from the console instead of lingering as a stale row.
func (s *Store) ReplaceGroupNodes(ctx context.Context, appID uuid.UUID, nodes []GroupNodeState) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	if _, err := tx.Exec(ctx, `DELETE FROM app_nodes WHERE app_id=$1`, appID); err != nil {
		return err
	}
	for _, n := range nodes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO app_nodes (app_id, node_address, status, execute_after, removal_initiator, synced_at)
			VALUES ($1, lower($2), $3, $4, lower(NULLIF($5,'')), now())`,
			appID, n.Address, n.Status, n.ExecuteAfter, n.Initiator); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func shortAddress(addr string) string {
	if len(addr) < 10 {
		return addr
	}
	return "Node " + addr[:6] + "…" + addr[len(addr)-4:]
}

func shortSlug(addr string) string {
	if len(addr) < 10 {
		return addr
	}
	return lower(addr[2:10])
}

// GroupComposition counts an app's active operators, split by whether they are
// run by O'Leary Labs.
//
// It reads the cached membership rather than the chain, so it is only as fresh
// as the last sync — callers that gate on it (promoting an app to production)
// sync first, so the answer reflects the group as it actually stands.
func (s *Store) GroupComposition(ctx context.Context, appID uuid.UUID, firstPartyCategory string) (total, firstParty, external int, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE n.category = $2),
			count(*) FILTER (WHERE n.category IS DISTINCT FROM $2)
		FROM app_nodes an
		LEFT JOIN node_operators n ON n.address = an.node_address
		WHERE an.app_id = $1 AND an.status = 'active'`,
		appID, firstPartyCategory,
	).Scan(&total, &firstParty, &external)
	return total, firstParty, external, err
}

// ExternalActiveOperators lists an app's active operators that are not run by
// O'Leary Labs, naming them so a refusal can say which ones it means.
//
// An operator the directory has never heard of counts as external: the platform
// cannot vouch for a node it has no record of, and treating an unknown as
// first-party would be the wrong way to be wrong.
func (s *Store) ExternalActiveOperators(ctx context.Context, appID uuid.UUID, firstPartyCategory string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT an.node_address, COALESCE(n.name, '')
		FROM app_nodes an
		LEFT JOIN node_operators n ON n.address = an.node_address
		WHERE an.app_id = $1 AND an.status = 'active'
		  AND (n.category IS NULL OR n.category <> $2)
		ORDER BY an.node_address`, appID, firstPartyCategory)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var addr, name string
		if err := rows.Scan(&addr, &name); err != nil {
			return nil, err
		}
		if name != "" {
			out = append(out, name)
		} else {
			out = append(out, addr)
		}
	}
	return out, rows.Err()
}

// FirstPartyOperatorAddresses lists the operators run by O'Leary Labs, which is
// the set a development group may draw from.
func (s *Store) FirstPartyOperatorAddresses(ctx context.Context, firstPartyCategory string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT address FROM node_operators WHERE category = $1 ORDER BY address`, firstPartyCategory)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// OrgPlan reads the plan of the organization that owns an app.
func (s *Store) OrgPlan(ctx context.Context, appID uuid.UUID) (string, error) {
	var plan string
	err := s.pool.QueryRow(ctx,
		`SELECT o.plan FROM apps a JOIN organizations o ON o.id = a.org_id WHERE a.id = $1`,
		appID).Scan(&plan)
	return plan, wrap(err)
}
