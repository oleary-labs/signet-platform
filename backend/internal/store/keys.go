package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/oleary-labs/signet-platform/backend/internal/structs"
)

const keyColumns = `
	id, key_id, curve, public_key, address, scope_hex, scope_kind, scope_chain_id,
	scope_contract, scope_type_hash, parent_key_id, threshold, parties, status,
	label, subject_hash, last_used_at, synced_at, created_at`

func scanKey(row interface{ Scan(...any) error }) (*structs.Key, error) {
	var k structs.Key
	err := row.Scan(&k.ID, &k.KeyID, &k.Curve, &k.PublicKey, &k.Address, &k.ScopeHex,
		&k.ScopeKind, &k.ScopeChainID, &k.ScopeContract, &k.ScopeTypeHash, &k.ParentKeyID,
		&k.Threshold, &k.Parties, &k.Status, &k.Label, &k.SubjectHash,
		&k.LastUsedAt, &k.SyncedAt, &k.CreatedAt)
	if err != nil {
		return nil, wrap(err)
	}
	return &k, nil
}

// KeyFilter narrows the wallet listing.
type KeyFilter struct {
	Curve       string
	ScopeKind   string
	Status      string
	SubjectHash string
	Search      string
	Limit       int
	Offset      int
}

// Keys lists an app's threshold keys.
func (s *Store) Keys(ctx context.Context, appID uuid.UUID, f KeyFilter) ([]structs.Key, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+keyColumns+` FROM app_keys
		WHERE app_id = $1
		  AND ($2 = '' OR curve = $2)
		  AND ($3 = '' OR scope_kind = $3)
		  AND ($4 = '' OR status = $4)
		  AND ($5 = '' OR subject_hash = $5)
		  AND ($6 = '' OR key_id ILIKE '%' || $6 || '%' OR address ILIKE '%' || $6 || '%' OR label ILIKE '%' || $6 || '%')
		ORDER BY created_at DESC
		LIMIT $7 OFFSET $8`,
		appID, f.Curve, f.ScopeKind, f.Status, f.SubjectHash, f.Search, f.Limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.Key{}
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

// Key loads one key by its protocol key ID and curve.
func (s *Store) Key(ctx context.Context, appID uuid.UUID, keyID, curve string) (*structs.Key, error) {
	return scanKey(s.pool.QueryRow(ctx,
		`SELECT `+keyColumns+` FROM app_keys WHERE app_id=$1 AND key_id=$2 AND curve=$3`,
		appID, keyID, curve))
}

// KeyRecord is one entry of a key-inventory snapshot.
//
// The snapshot is fetched by the developer's own browser (which holds the auth
// key the node requires) and posted here to cache. The platform cannot fetch
// it itself, because it deliberately holds no credential that a node would
// accept — which is the whole point of the trust model.
type KeyRecord struct {
	KeyID         string
	Curve         string
	PublicKey     string
	Address       string
	ScopeHex      string
	ScopeKind     string
	ScopeChainID  *int64
	ScopeContract string
	ScopeTypeHash string
	ParentKeyID   string
	Threshold     int
	Parties       []string
	Status        string
	SubjectHash   string
}

// SyncKeys reconciles an app's cached key inventory with a snapshot.
//
// Keys absent from the snapshot are marked deleted rather than dropped, so a
// key that stops appearing (because it was deleted on the nodes, or because
// the snapshot was partial) leaves a visible trace instead of vanishing.
func (s *Store) SyncKeys(ctx context.Context, appID uuid.UUID, records []KeyRecord) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	seen := make([]string, 0, len(records))
	for _, r := range records {
		if r.Parties == nil {
			r.Parties = []string{}
		}
		if r.ScopeKind == "" {
			r.ScopeKind = "unscoped"
		}
		if r.Status == "" {
			r.Status = "enabled"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO app_keys
				(app_id, key_id, curve, public_key, address, scope_hex, scope_kind,
				 scope_chain_id, scope_contract, scope_type_hash, parent_key_id,
				 threshold, parties, status, subject_hash, synced_at)
			VALUES ($1, $2, $3, NULLIF($4,''), lower(NULLIF($5,'')), NULLIF($6,''), $7,
			        $8, lower(NULLIF($9,'')), NULLIF($10,''), NULLIF($11,''),
			        NULLIF($12,0), $13, $14, NULLIF($15,''), now())
			ON CONFLICT (app_id, key_id, curve) DO UPDATE SET
				public_key      = COALESCE(EXCLUDED.public_key, app_keys.public_key),
				address         = COALESCE(EXCLUDED.address, app_keys.address),
				scope_hex       = COALESCE(EXCLUDED.scope_hex, app_keys.scope_hex),
				scope_kind      = EXCLUDED.scope_kind,
				scope_chain_id  = COALESCE(EXCLUDED.scope_chain_id, app_keys.scope_chain_id),
				scope_contract  = COALESCE(EXCLUDED.scope_contract, app_keys.scope_contract),
				scope_type_hash = COALESCE(EXCLUDED.scope_type_hash, app_keys.scope_type_hash),
				parent_key_id   = COALESCE(EXCLUDED.parent_key_id, app_keys.parent_key_id),
				threshold       = COALESCE(EXCLUDED.threshold, app_keys.threshold),
				parties         = EXCLUDED.parties,
				status          = EXCLUDED.status,
				subject_hash    = COALESCE(EXCLUDED.subject_hash, app_keys.subject_hash),
				synced_at       = now()`,
			appID, r.KeyID, r.Curve, r.PublicKey, r.Address, r.ScopeHex, r.ScopeKind,
			r.ScopeChainID, r.ScopeContract, r.ScopeTypeHash, r.ParentKeyID,
			r.Threshold, r.Parties, r.Status, r.SubjectHash); err != nil {
			return 0, err
		}
		seen = append(seen, r.KeyID+"|"+r.Curve)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE app_keys SET status='deleted', synced_at=now()
		WHERE app_id=$1 AND status <> 'deleted' AND (key_id || '|' || curve) <> ALL($2)`,
		appID, seen); err != nil {
		return 0, err
	}

	// Roll the key counts on the end-user records forward in the same
	// transaction, so the users screen never disagrees with the keys screen.
	if _, err := tx.Exec(ctx, `
		INSERT INTO app_users (app_id, subject_hash, key_count, last_seen_at)
		SELECT $1, subject_hash, count(*), now()
		FROM app_keys
		WHERE app_id=$1 AND subject_hash IS NOT NULL AND status <> 'deleted'
		GROUP BY subject_hash
		ON CONFLICT (app_id, subject_hash) DO UPDATE SET
			key_count    = EXCLUDED.key_count,
			last_seen_at = GREATEST(app_users.last_seen_at, EXCLUDED.last_seen_at)`,
		appID); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(records), nil
}

// SetKeyLabel attaches a developer-facing name to a key. Labels are the
// platform's own metadata — the protocol neither stores nor sees them.
func (s *Store) SetKeyLabel(ctx context.Context, appID uuid.UUID, keyID, curve, label string) (*structs.Key, error) {
	return scanKey(s.pool.QueryRow(ctx,
		`UPDATE app_keys SET label=$4 WHERE app_id=$1 AND key_id=$2 AND curve=$3
		 RETURNING `+keyColumns, appID, keyID, curve, label))
}

// SetKeyStatus records a disable/enable that the console performed against the
// nodes. The nodes are authoritative; this only keeps the console honest until
// the next sync.
func (s *Store) SetKeyStatus(ctx context.Context, appID uuid.UUID, keyID, curve, status string) (*structs.Key, error) {
	return scanKey(s.pool.QueryRow(ctx,
		`UPDATE app_keys SET status=$4, synced_at=now()
		 WHERE app_id=$1 AND key_id=$2 AND curve=$3
		 RETURNING `+keyColumns, appID, keyID, curve, status))
}

// KeyStats is the wallet screen's summary strip.
type KeyStats struct {
	Total    int            `json:"total"`
	Enabled  int            `json:"enabled"`
	Disabled int            `json:"disabled"`
	ByCurve  map[string]int `json:"by_curve"`
	ByScope  map[string]int `json:"by_scope"`
}

// KeyStats counts an app's keys by status, curve, and scope.
func (s *Store) KeyStats(ctx context.Context, appID uuid.UUID) (*KeyStats, error) {
	st := &KeyStats{ByCurve: map[string]int{}, ByScope: map[string]int{}}
	rows, err := s.pool.Query(ctx,
		`SELECT curve, scope_kind, status, count(*) FROM app_keys WHERE app_id=$1
		 GROUP BY curve, scope_kind, status`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var curve, scope, status string
		var n int
		if err := rows.Scan(&curve, &scope, &status, &n); err != nil {
			return nil, err
		}
		if status == "deleted" {
			continue
		}
		st.Total += n
		if status == "enabled" {
			st.Enabled += n
		} else {
			st.Disabled += n
		}
		st.ByCurve[curve] += n
		st.ByScope[scope] += n
	}
	return st, rows.Err()
}

// ─────────────────────────── End users ───────────────────────────

// AppUsers lists an app's end users, newest activity first.
func (s *Store) AppUsers(ctx context.Context, appID uuid.UUID, search string, limit, offset int) ([]structs.AppUser, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, subject_hash, issuer, label, key_count, status, first_seen_at, last_seen_at
		FROM app_users
		WHERE app_id=$1
		  AND ($2 = '' OR subject_hash ILIKE '%' || $2 || '%' OR label ILIKE '%' || $2 || '%')
		ORDER BY last_seen_at DESC
		LIMIT $3 OFFSET $4`, appID, search, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.AppUser{}
	for rows.Next() {
		var u structs.AppUser
		if err := rows.Scan(&u.ID, &u.SubjectHash, &u.Issuer, &u.Label, &u.KeyCount,
			&u.Status, &u.FirstSeenAt, &u.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// TouchAppUser records activity for an end user, creating the row on first
// sight. Called from the metering ingest path.
func (s *Store) TouchAppUser(ctx context.Context, appID uuid.UUID, subjectHash, issuer string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO app_users (app_id, subject_hash, issuer)
		VALUES ($1, $2, $3)
		ON CONFLICT (app_id, subject_hash) DO UPDATE SET
			last_seen_at = now(),
			issuer = COALESCE(NULLIF(EXCLUDED.issuer, ''), app_users.issuer)`,
		appID, subjectHash, issuer)
	return err
}

// SetAppUserLabel attaches a developer-supplied label to an end user.
func (s *Store) SetAppUserLabel(ctx context.Context, appID, userID uuid.UUID, label string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE app_users SET label=$3 WHERE app_id=$1 AND id=$2`, appID, userID, label)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ─────────────────────────── Delegations ───────────────────────────

// Delegations lists an app's session signers.
func (s *Store) Delegations(ctx context.Context, appID uuid.UUID, includeRevoked bool) ([]structs.Delegation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, key_id, parent_key_id, curve, label, subject_hash, issued_at, expires_at, revoked_at
		FROM delegations
		WHERE app_id=$1 AND ($2 OR revoked_at IS NULL)
		ORDER BY issued_at DESC`, appID, includeRevoked)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.Delegation{}
	for rows.Next() {
		var d structs.Delegation
		if err := rows.Scan(&d.ID, &d.KeyID, &d.ParentKeyID, &d.Curve, &d.Label,
			&d.SubjectHash, &d.IssuedAt, &d.ExpiresAt, &d.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// RecordDelegation stores the metadata of a delegation the console minted.
// The token itself is never sent here — only a hash, so a later presentation
// can be matched to this row without the platform ever holding a usable one.
func (s *Store) RecordDelegation(ctx context.Context, appID, createdBy uuid.UUID, keyID, parentKeyID, curve, label, subjectHash, tokenHash string, expiresAt time.Time) (*structs.Delegation, error) {
	var d structs.Delegation
	err := s.pool.QueryRow(ctx, `
		INSERT INTO delegations (app_id, key_id, parent_key_id, curve, label, subject_hash, token_hash, expires_at, created_by)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6,''), NULLIF($7,''), $8, $9)
		RETURNING id, key_id, parent_key_id, curve, label, subject_hash, issued_at, expires_at, revoked_at`,
		appID, keyID, parentKeyID, curve, label, subjectHash, tokenHash, expiresAt, createdBy,
	).Scan(&d.ID, &d.KeyID, &d.ParentKeyID, &d.Curve, &d.Label, &d.SubjectHash,
		&d.IssuedAt, &d.ExpiresAt, &d.RevokedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// RevokeDelegation marks a delegation revoked in the platform's record.
//
// This does not invalidate the token: a delegation JWT is verified by the
// nodes against the parent key, and the only way to actually stop it is to
// disable the sub-key on the nodes. The console pairs the two actions and says
// so, rather than implying a database row can revoke a signature.
func (s *Store) RevokeDelegation(ctx context.Context, appID, delegationID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE delegations SET revoked_at=now()
		 WHERE id=$1 AND app_id=$2 AND revoked_at IS NULL`, delegationID, appID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ─────────────────────────── Policies ───────────────────────────

// Policies lists an app's rules.
func (s *Store) Policies(ctx context.Context, appID uuid.UUID) ([]structs.Policy, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, kind, enforced_by, config, enabled, created_at, updated_at
		FROM policies WHERE app_id=$1 ORDER BY created_at`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.Policy{}
	for rows.Next() {
		var p structs.Policy
		if err := rows.Scan(&p.ID, &p.Name, &p.Kind, &p.EnforcedBy, &p.Config,
			&p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpsertPolicy creates or replaces a named policy.
func (s *Store) UpsertPolicy(ctx context.Context, appID, createdBy uuid.UUID, name, kind, enforcedBy string, config json.RawMessage, enabled bool) (*structs.Policy, error) {
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	var p structs.Policy
	err := s.pool.QueryRow(ctx, `
		INSERT INTO policies (app_id, name, kind, enforced_by, config, enabled, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (app_id, name) DO UPDATE SET
			kind = EXCLUDED.kind, enforced_by = EXCLUDED.enforced_by,
			config = EXCLUDED.config, enabled = EXCLUDED.enabled, updated_at = now()
		RETURNING id, name, kind, enforced_by, config, enabled, created_at, updated_at`,
		appID, name, kind, enforcedBy, config, enabled, createdBy,
	).Scan(&p.ID, &p.Name, &p.Kind, &p.EnforcedBy, &p.Config, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DeletePolicy removes a rule.
func (s *Store) DeletePolicy(ctx context.Context, appID, policyID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM policies WHERE id=$1 AND app_id=$2`, policyID, appID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
