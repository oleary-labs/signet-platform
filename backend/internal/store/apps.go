package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/oleary-labs/signet-platform/backend/internal/structs"
)

const appColumns = `
	a.id, a.org_id, a.name, a.slug, a.description, a.environment, a.status, a.chain_id,
	a.group_address, a.group_public_key, a.threshold, a.node_count, a.is_operational,
	a.logo_url, a.website_url, a.deployed_at, a.synced_at, a.created_at, a.updated_at`

func scanApp(row interface{ Scan(...any) error }) (*structs.App, error) {
	var a structs.App
	err := row.Scan(&a.ID, &a.OrgID, &a.Name, &a.Slug, &a.Description, &a.Environment, &a.Status,
		&a.ChainID, &a.GroupAddress, &a.GroupPublicKey, &a.Threshold, &a.NodeCount, &a.IsOperational,
		&a.LogoURL, &a.WebsiteURL, &a.DeployedAt, &a.SyncedAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, wrap(err)
	}
	return &a, nil
}

// Apps lists an organization's live apps.
func (s *Store) Apps(ctx context.Context, orgID uuid.UUID) ([]structs.App, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+appColumns+` FROM apps a WHERE a.org_id=$1 AND a.archived_at IS NULL ORDER BY a.created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.App{}
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// AppsForUser lists every app across every org the user belongs to — the
// console's top-level app switcher.
func (s *Store) AppsForUser(ctx context.Context, userID uuid.UUID) ([]structs.App, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+appColumns+`
		FROM apps a
		JOIN org_members m ON m.org_id = a.org_id AND m.user_id = $1
		WHERE a.archived_at IS NULL
		ORDER BY a.updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.App{}
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// App loads one app.
func (s *Store) App(ctx context.Context, appID uuid.UUID) (*structs.App, error) {
	return scanApp(s.pool.QueryRow(ctx,
		`SELECT `+appColumns+` FROM apps a WHERE a.id=$1`, appID))
}

// AppByGroup finds the app backed by a given signing group, used when the
// chain indexer discovers a group the platform already knows about.
func (s *Store) AppByGroup(ctx context.Context, groupAddress string) (*structs.App, error) {
	return scanApp(s.pool.QueryRow(ctx,
		`SELECT `+appColumns+` FROM apps a WHERE lower(a.group_address)=lower($1)`, groupAddress))
}

// CreateAppInput is what the creation wizard submits before deployment.
type CreateAppInput struct {
	OrgID       uuid.UUID
	CreatedBy   uuid.UUID
	Name        string
	Description string
	Environment string
	ChainID     int64
	WebsiteURL  string
	LogoURL     string
}

// CreateApp records a new app in the `provisioning` state and seeds its
// settings row. The app has no group yet: the group is deployed by the
// developer's own UserOperation, and AttachGroup binds it afterwards.
func (s *Store) CreateApp(ctx context.Context, in CreateAppInput) (*structs.App, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	slug, err := freeAppSlug(ctx, tx, in.OrgID, Slugify(in.Name))
	if err != nil {
		return nil, err
	}

	a, err := scanApp(tx.QueryRow(ctx, `
		INSERT INTO apps (org_id, name, slug, description, environment, chain_id, website_url, logo_url, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7,''), NULLIF($8,''), $9)
		RETURNING `+strippedAppColumns,
		in.OrgID, in.Name, slug, in.Description, in.Environment, in.ChainID, in.WebsiteURL, in.LogoURL, in.CreatedBy))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, err
	}

	if _, err := tx.Exec(ctx, `INSERT INTO app_settings (app_id) VALUES ($1)`, a.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

// strippedAppColumns is appColumns without the `a.` qualifier, for RETURNING
// clauses where there is no table alias in scope.
const strippedAppColumns = `
	id, org_id, name, slug, description, environment, status, chain_id,
	group_address, group_public_key, threshold, node_count, is_operational,
	logo_url, website_url, deployed_at, synced_at, created_at, updated_at`

// UpdateAppInput carries the editable fields; empty strings mean "leave as is".
type UpdateAppInput struct {
	Name        *string
	Description *string
	Environment *string
	WebsiteURL  *string
	LogoURL     *string
}

// UpdateApp edits an app's descriptive fields. The group binding and every
// mirrored chain field are excluded on purpose — those come from the chain.
func (s *Store) UpdateApp(ctx context.Context, appID uuid.UUID, in UpdateAppInput) (*structs.App, error) {
	return scanApp(s.pool.QueryRow(ctx, `
		UPDATE apps SET
			name        = COALESCE($2, name),
			description = COALESCE($3, description),
			environment = COALESCE($4, environment),
			website_url = COALESCE($5, website_url),
			logo_url    = COALESCE($6, logo_url),
			updated_at  = now()
		WHERE id=$1 AND archived_at IS NULL
		RETURNING `+strippedAppColumns,
		appID, in.Name, in.Description, in.Environment, in.WebsiteURL, in.LogoURL))
}

// AttachGroup binds a deployed signing group to an app and marks it live.
// The unique index on group_address means one group can back only one app, so
// a copy-pasted address cannot silently hijack another org's group.
func (s *Store) AttachGroup(ctx context.Context, appID uuid.UUID, groupAddress, groupPublicKey string, threshold, nodeCount int) (*structs.App, error) {
	a, err := scanApp(s.pool.QueryRow(ctx, `
		UPDATE apps SET
			group_address    = lower($2),
			group_public_key = NULLIF($3,''),
			threshold        = $4,
			node_count       = $5,
			status           = 'live',
			deployed_at      = COALESCE(deployed_at, now()),
			updated_at       = now()
		WHERE id=$1 AND archived_at IS NULL
		RETURNING `+strippedAppColumns,
		appID, groupAddress, groupPublicKey, threshold, nodeCount))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: that signing group is already attached to another app", ErrConflict)
		}
		return nil, err
	}
	return a, nil
}

// SyncGroupState refreshes the chain-derived summary fields on an app.
func (s *Store) SyncGroupState(ctx context.Context, appID uuid.UUID, threshold, activeNodes int, operational bool) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE apps SET threshold=$2, node_count=$3, is_operational=$4, synced_at=now()
		WHERE id=$1`, appID, threshold, activeNodes, operational)
	return err
}

// ArchiveApp soft-deletes an app. Nothing on-chain is touched: the signing
// group keeps running and keeps serving the developer's users until they
// remove its nodes themselves. Saying so plainly in the console matters more
// than pretending the platform can switch a group off.
func (s *Store) ArchiveApp(ctx context.Context, appID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE apps SET archived_at=now(), status='archived', updated_at=now()
		 WHERE id=$1 AND archived_at IS NULL`, appID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ─────────────────────────── Settings ───────────────────────────

// Settings loads an app's configuration documents, creating the row lazily for
// apps that predate it.
func (s *Store) Settings(ctx context.Context, appID uuid.UUID) (*structs.AppSettings, error) {
	var st structs.AppSettings
	err := s.pool.QueryRow(ctx, `
		INSERT INTO app_settings (app_id) VALUES ($1)
		ON CONFLICT (app_id) DO UPDATE SET app_id = EXCLUDED.app_id
		RETURNING login_methods, branding, embedded_wallets, smart_accounts,
		          session_signers, funding, compliance, updated_at`, appID,
	).Scan(&st.LoginMethods, &st.Branding, &st.EmbeddedWallets, &st.SmartAccounts,
		&st.SessionSigners, &st.Funding, &st.Compliance, &st.UpdatedAt)
	if err != nil {
		return nil, wrap(err)
	}
	return &st, nil
}

// settingsSections maps a request key to its column. Restricting updates to
// this map is what keeps the section name out of the SQL string.
var settingsSections = map[string]string{
	"login_methods":    "login_methods",
	"branding":         "branding",
	"embedded_wallets": "embedded_wallets",
	"smart_accounts":   "smart_accounts",
	"session_signers":  "session_signers",
	"funding":          "funding",
	"compliance":       "compliance",
}

// UpdateSettings replaces one configuration section. Sections are written
// individually so two people editing different screens do not overwrite each
// other's work.
func (s *Store) UpdateSettings(ctx context.Context, appID uuid.UUID, section string, doc json.RawMessage) (*structs.AppSettings, error) {
	column, ok := settingsSections[section]
	if !ok {
		return nil, fmt.Errorf("unknown settings section %q", section)
	}
	if !json.Valid(doc) {
		return nil, fmt.Errorf("settings section %q is not valid JSON", section)
	}
	// The column name comes from the map above, never from the request, so the
	// only interpolated value here is one of seven fixed identifiers.
	query := fmt.Sprintf(`
		INSERT INTO app_settings (app_id, %s) VALUES ($1, $2)
		ON CONFLICT (app_id) DO UPDATE SET %s = EXCLUDED.%s, updated_at = now()`, column, column, column)
	if _, err := s.pool.Exec(ctx, query, appID, doc); err != nil {
		return nil, err
	}
	return s.Settings(ctx, appID)
}

// ─────────────────────────── Domains ───────────────────────────

// Domains lists the origins allowed to use an app's client SDK.
func (s *Store) Domains(ctx context.Context, appID uuid.UUID) ([]structs.AppDomain, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, origin, verified_at, created_at FROM app_domains WHERE app_id=$1 ORDER BY created_at`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.AppDomain{}
	for rows.Next() {
		var d structs.AppDomain
		if err := rows.Scan(&d.ID, &d.Origin, &d.VerifiedAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AddDomain allows an origin.
func (s *Store) AddDomain(ctx context.Context, appID uuid.UUID, origin string) (*structs.AppDomain, error) {
	var d structs.AppDomain
	err := s.pool.QueryRow(ctx,
		`INSERT INTO app_domains (app_id, origin) VALUES ($1, $2)
		 RETURNING id, origin, verified_at, created_at`, appID, origin,
	).Scan(&d.ID, &d.Origin, &d.VerifiedAt, &d.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, err
	}
	return &d, nil
}

// RemoveDomain revokes an origin.
func (s *Store) RemoveDomain(ctx context.Context, appID, domainID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM app_domains WHERE id=$1 AND app_id=$2`, domainID, appID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ─────────────────────────── Credentials ───────────────────────────

// Credentials lists an app's active credentials.
func (s *Store) Credentials(ctx context.Context, appID uuid.UUID) ([]structs.Credential, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, kind, label, last_four, public_key, key_hash, onchain_status, last_used_at, created_at
		FROM app_credentials
		WHERE app_id=$1 AND revoked_at IS NULL
		ORDER BY created_at DESC`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.Credential{}
	for rows.Next() {
		var c structs.Credential
		if err := rows.Scan(&c.ID, &c.Kind, &c.Label, &c.LastFour, &c.PublicKey,
			&c.KeyHash, &c.OnchainStatus, &c.LastUsedAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateAppSecret mints a platform API secret and returns it once. Only a hash
// is stored, so a later request can verify a presented secret but nobody —
// including platform staff — can read it back.
func (s *Store) CreateAppSecret(ctx context.Context, appID, createdBy uuid.UUID, label string) (*structs.Credential, error) {
	secret, err := RandomToken(32)
	if err != nil {
		return nil, err
	}
	full := "sk_signet_" + secret
	lastFour := full[len(full)-4:]

	var c structs.Credential
	err = s.pool.QueryRow(ctx, `
		INSERT INTO app_credentials (app_id, kind, label, secret_hash, last_four, created_by)
		VALUES ($1, 'app_secret', $2, $3, $4, $5)
		RETURNING id, kind, label, last_four, public_key, key_hash, onchain_status, last_used_at, created_at`,
		appID, label, HashToken(full), lastFour, createdBy,
	).Scan(&c.ID, &c.Kind, &c.Label, &c.LastFour, &c.PublicKey, &c.KeyHash,
		&c.OnchainStatus, &c.LastUsedAt, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	c.Secret = full
	return &c, nil
}

// RecordAuthKey mirrors an on-chain authorization key. The platform stores the
// public half only: the private key is generated in the developer's browser,
// shown once, and never transmitted here.
func (s *Store) RecordAuthKey(ctx context.Context, appID, createdBy uuid.UUID, label, publicKey, keyHash, status string) (*structs.Credential, error) {
	var c structs.Credential
	err := s.pool.QueryRow(ctx, `
		INSERT INTO app_credentials (app_id, kind, label, public_key, key_hash, onchain_status, created_by)
		VALUES ($1, 'auth_key', $2, $3, $4, $5, $6)
		RETURNING id, kind, label, last_four, public_key, key_hash, onchain_status, last_used_at, created_at`,
		appID, label, publicKey, keyHash, status, createdBy,
	).Scan(&c.ID, &c.Kind, &c.Label, &c.LastFour, &c.PublicKey, &c.KeyHash,
		&c.OnchainStatus, &c.LastUsedAt, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// RevokeCredential marks a credential revoked. For an auth key this only
// updates the platform's record — removing it from the group is a separate
// on-chain call the developer makes, and the console says so.
func (s *Store) RevokeCredential(ctx context.Context, appID, credentialID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE app_credentials SET revoked_at=now()
		 WHERE id=$1 AND app_id=$2 AND revoked_at IS NULL`, credentialID, appID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// VerifyAppSecret resolves a presented app secret to its app, and records the
// use. It returns ErrNotFound for an unknown or revoked secret.
func (s *Store) VerifyAppSecret(ctx context.Context, secret string) (uuid.UUID, error) {
	var appID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		UPDATE app_credentials SET last_used_at = now()
		WHERE kind='app_secret' AND secret_hash=$1 AND revoked_at IS NULL
		RETURNING app_id`, HashToken(secret)).Scan(&appID)
	if err != nil {
		return uuid.Nil, wrap(err)
	}
	return appID, nil
}

// SyncAuthKeyStatuses reconciles the platform's mirror of an app's on-chain
// auth keys with what the group contract actually holds.
func (s *Store) SyncAuthKeyStatuses(ctx context.Context, appID uuid.UUID, onchainKeys []string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE app_credentials SET
			onchain_status = CASE WHEN lower(public_key) = ANY($2) THEN 'active' ELSE 'removed' END
		WHERE app_id=$1 AND kind='auth_key' AND revoked_at IS NULL`,
		appID, lowerAll(onchainKeys))
	return err
}

// ─────────────────────────── Issuers ───────────────────────────

// Issuers lists an app's configured login issuers.
func (s *Store) Issuers(ctx context.Context, appID uuid.UUID) ([]structs.Issuer, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, issuer, issuer_hash, client_ids, provider, label, enabled, onchain_status, created_at
		FROM app_issuers WHERE app_id=$1 ORDER BY created_at`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.Issuer{}
	for rows.Next() {
		var i structs.Issuer
		if err := rows.Scan(&i.ID, &i.Issuer, &i.IssuerHash, &i.ClientIDs, &i.Provider,
			&i.Label, &i.Enabled, &i.OnchainStatus, &i.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// UpsertIssuer records an issuer and its allowed client IDs.
//
// An empty client-ID list means "any client from this issuer", matching the
// group contract's semantics. A list containing an empty string is a different
// thing entirely — it matches nothing — so callers must not pad the list.
func (s *Store) UpsertIssuer(ctx context.Context, appID uuid.UUID, issuer, issuerHash string, clientIDs []string, provider, label string) (*structs.Issuer, error) {
	if clientIDs == nil {
		clientIDs = []string{}
	}
	var i structs.Issuer
	err := s.pool.QueryRow(ctx, `
		INSERT INTO app_issuers (app_id, issuer, issuer_hash, client_ids, provider, label)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (app_id, issuer) DO UPDATE SET
			issuer_hash = EXCLUDED.issuer_hash,
			client_ids  = EXCLUDED.client_ids,
			provider    = EXCLUDED.provider,
			label       = EXCLUDED.label,
			enabled     = true
		RETURNING id, issuer, issuer_hash, client_ids, provider, label, enabled, onchain_status, created_at`,
		appID, issuer, issuerHash, clientIDs, provider, label,
	).Scan(&i.ID, &i.Issuer, &i.IssuerHash, &i.ClientIDs, &i.Provider, &i.Label,
		&i.Enabled, &i.OnchainStatus, &i.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// RemoveIssuer deletes the platform's record of an issuer.
func (s *Store) RemoveIssuer(ctx context.Context, appID, issuerID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM app_issuers WHERE id=$1 AND app_id=$2`, issuerID, appID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AdoptIssuer records an issuer found on the group contract that the platform
// has no row for — a group created outside this console, or an issuer added
// directly on-chain.
//
// Without this the console would show "no login methods configured" for a group
// that has one, which is both wrong and alarming. An existing row is left
// alone so a developer's label and provider choice survive the sync.
func (s *Store) AdoptIssuer(ctx context.Context, appID uuid.UUID, issuer, issuerHash string, clientIDs []string, provider string) error {
	if clientIDs == nil {
		clientIDs = []string{}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO app_issuers (app_id, issuer, issuer_hash, client_ids, provider, onchain_status, synced_at)
		VALUES ($1, $2, $3, $4, $5, 'active', now())
		ON CONFLICT (app_id, issuer) DO UPDATE SET
			client_ids     = EXCLUDED.client_ids,
			issuer_hash    = COALESCE(app_issuers.issuer_hash, EXCLUDED.issuer_hash),
			onchain_status = 'active',
			synced_at      = now()`,
		appID, issuer, issuerHash, clientIDs, provider)
	return err
}

// SyncIssuerStatuses marks the mirror's issuers active or removed according to
// what the group contract actually holds. Adoption of unknown issuers happens
// separately, in AdoptIssuer.
func (s *Store) SyncIssuerStatuses(ctx context.Context, appID uuid.UUID, onchain []string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE app_issuers SET
			onchain_status = CASE WHEN issuer = ANY($2) THEN 'active' ELSE 'removed' END,
			synced_at      = now()
		WHERE app_id=$1`, appID, onchain)
	return err
}

// AdoptAuthKey records an authorization key the group trusts that the platform
// has no row for.
//
// Surfacing it matters more than tidiness: a key trusted by the group that
// nobody in the console recognises is exactly the thing a developer should be
// looking at, and hiding it would make the credentials screen quietly
// incomplete.
func (s *Store) AdoptAuthKey(ctx context.Context, appID uuid.UUID, publicKey, keyHash string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO app_credentials (app_id, kind, label, public_key, key_hash, onchain_status)
		SELECT $1, 'auth_key', 'Discovered on-chain', lower($2), $3, 'active'
		WHERE NOT EXISTS (
			SELECT 1 FROM app_credentials
			WHERE app_id=$1 AND kind='auth_key' AND lower(public_key)=lower($2)
		)`, appID, publicKey, keyHash)
	return err
}

// freeAppSlug finds a slug unused within one organization.
func freeAppSlug(ctx context.Context, q querier, orgID uuid.UUID, base string) (string, error) {
	for i := 1; i <= 50; i++ {
		candidate := base
		if i > 1 {
			candidate = fmt.Sprintf("%s-%d", base, i)
		}
		var exists bool
		if err := q.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM apps WHERE org_id=$1 AND slug=$2)`, orgID, candidate).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not find a free slug for %q", base)
}

func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = lower(s)
	}
	return out
}

// TouchApp bumps updated_at so an app sorts to the top of the switcher after
// any meaningful change.
func (s *Store) TouchApp(ctx context.Context, appID uuid.UUID) {
	_, _ = s.pool.Exec(ctx, `UPDATE apps SET updated_at=now() WHERE id=$1`, appID)
}
