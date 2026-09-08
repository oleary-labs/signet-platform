package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/oleary-labs/signet-platform/backend/internal/structs"
)

// UpsertUser resolves a login to a user row, creating it on first contact.
//
// `subject` is the stable identifier for the login route: the SignetAccount
// address for the dogfooded FROST route, the EOA for SIWE. A user who signs in
// both ways therefore has two rows — deliberately, because proving control of
// an EOA is not proof of control of a Signet account.
func (s *Store) UpsertUser(ctx context.Context, subject, kind, accountAddress, groupPublicKey string, isStaff bool) (*structs.User, error) {
	var u structs.User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (subject, subject_kind, account_address, group_public_key, is_staff, last_seen_at)
		VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), $5, now())
		ON CONFLICT (subject) DO UPDATE SET
			last_seen_at     = now(),
			updated_at       = now(),
			account_address  = COALESCE(NULLIF(EXCLUDED.account_address,''), users.account_address),
			group_public_key = COALESCE(NULLIF(EXCLUDED.group_public_key,''), users.group_public_key),
			-- Staff is granted by configuration, never revoked by a login.
			is_staff         = users.is_staff OR EXCLUDED.is_staff
		RETURNING id, subject, subject_kind, account_address, group_public_key,
		          email::text, display_name, avatar_url, is_staff, last_seen_at, created_at,
		          smart_account_address, signet_key_id, signet_key_address`,
		subject, kind, accountAddress, groupPublicKey, isStaff,
	).Scan(&u.ID, &u.Subject, &u.SubjectKind, &u.AccountAddress, &u.GroupPublicKey,
		&u.Email, &u.DisplayName, &u.AvatarURL, &u.IsStaff, &u.LastSeenAt, &u.CreatedAt,
		&u.SmartAccountAddress, &u.SignetKeyID, &u.SignetKeyAddress)
	if err != nil {
		return nil, wrap(err)
	}
	return &u, nil
}

// User loads one user by id.
func (s *Store) User(ctx context.Context, id uuid.UUID) (*structs.User, error) {
	var u structs.User
	err := s.pool.QueryRow(ctx, `
		SELECT id, subject, subject_kind, account_address, group_public_key,
		       email::text, display_name, avatar_url, is_staff, last_seen_at, created_at,
		          smart_account_address, signet_key_id, signet_key_address
		FROM users WHERE id=$1`, id,
	).Scan(&u.ID, &u.Subject, &u.SubjectKind, &u.AccountAddress, &u.GroupPublicKey,
		&u.Email, &u.DisplayName, &u.AvatarURL, &u.IsStaff, &u.LastSeenAt, &u.CreatedAt,
		&u.SmartAccountAddress, &u.SignetKeyID, &u.SignetKeyAddress)
	if err != nil {
		return nil, wrap(err)
	}
	return &u, nil
}

// UpdateProfile sets the display fields a developer controls.
func (s *Store) UpdateProfile(ctx context.Context, id uuid.UUID, displayName, email, avatarURL string) (*structs.User, error) {
	var u structs.User
	err := s.pool.QueryRow(ctx, `
		UPDATE users SET
			display_name = COALESCE(NULLIF($2,''), display_name),
			email        = COALESCE(NULLIF($3,'')::citext, email),
			avatar_url   = COALESCE(NULLIF($4,''), avatar_url),
			updated_at   = now()
		WHERE id=$1
		RETURNING id, subject, subject_kind, account_address, group_public_key,
		          email::text, display_name, avatar_url, is_staff, last_seen_at, created_at,
		          smart_account_address, signet_key_id, signet_key_address`,
		id, displayName, email, avatarURL,
	).Scan(&u.ID, &u.Subject, &u.SubjectKind, &u.AccountAddress, &u.GroupPublicKey,
		&u.Email, &u.DisplayName, &u.AvatarURL, &u.IsStaff, &u.LastSeenAt, &u.CreatedAt,
		&u.SmartAccountAddress, &u.SignetKeyID, &u.SignetKeyAddress)
	if err != nil {
		return nil, wrap(err)
	}
	return &u, nil
}

// SyncStaffBySubject makes the users table agree with the configured staff
// list, granting to subjects that are on it and revoking from those that are
// not.
//
// Revocation is the half worth being deliberate about. Sign-in can only ever
// grant — its upsert ORs the flag so a login never clears rights someone was
// given — which means removing a subject from the configuration would
// otherwise leave them staff forever. Reconciling here makes the environment
// authoritative, and an empty list correctly means nobody is staff.
//
// Returns the counts separately so a boot that quietly removes someone's
// rights says so in the log.
func (s *Store) SyncStaffBySubject(ctx context.Context, subjects []string) (granted, revoked int64, err error) {
	if subjects == nil {
		subjects = []string{}
	}
	grant, err := s.pool.Exec(ctx,
		`UPDATE users SET is_staff=true, updated_at=now()
		 WHERE lower(subject) = ANY($1) AND is_staff=false`, subjects)
	if err != nil {
		return 0, 0, err
	}
	revoke, err := s.pool.Exec(ctx,
		`UPDATE users SET is_staff=false, updated_at=now()
		 WHERE NOT (lower(subject) = ANY($1)) AND is_staff=true`, subjects)
	if err != nil {
		return grant.RowsAffected(), 0, err
	}
	return grant.RowsAffected(), revoke.RowsAffected(), nil
}

// ─────────────────────────── Login challenges ───────────────────────────

// CreateChallenge records a single-use login nonce.
func (s *Store) CreateChallenge(ctx context.Context, nonce, method, address, statement string, ttl time.Duration) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth_challenges (nonce, method, address, statement, expires_at)
		VALUES ($1, $2, NULLIF($3,''), $4, now() + $5::interval)`,
		nonce, method, address, statement, ttl.String())
	return err
}

// ConsumeChallenge atomically marks a nonce used and reports whether it was
// valid. The UPDATE is the check: two concurrent verifications of the same
// nonce cannot both succeed, so a captured challenge cannot be replayed.
func (s *Store) ConsumeChallenge(ctx context.Context, nonce, method string) (address string, err error) {
	var addr *string
	err = s.pool.QueryRow(ctx, `
		UPDATE auth_challenges
		SET consumed_at = now()
		WHERE nonce = $1
		  AND method = $2
		  AND consumed_at IS NULL
		  AND expires_at > now()
		RETURNING address`, nonce, method).Scan(&addr)
	if err != nil {
		return "", wrap(err)
	}
	if addr != nil {
		address = *addr
	}
	return address, nil
}

// PurgeExpiredChallenges drops consumed and expired nonces. Rows are kept for
// an hour past expiry so a replay attempt is still diagnosable in the logs.
func (s *Store) PurgeExpiredChallenges(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM auth_challenges WHERE expires_at < now() - interval '1 hour'`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// RecordSignetKey stores the Signet key a developer's console generated and the
// smart wallet it controls.
//
// The smart account is unique across users: two developers cannot end up
// pointing at the same wallet, which would let either of them act as the other.
// A conflict surfaces as an error rather than silently overwriting.
func (s *Store) RecordSignetKey(ctx context.Context, userID uuid.UUID, groupPublicKey, keyID, keyAddress, smartAccount string) (*structs.User, error) {
	var u structs.User
	err := s.pool.QueryRow(ctx, `
		UPDATE users SET
			group_public_key      = $2,
			signet_key_id         = COALESCE(NULLIF($3,''), signet_key_id),
			signet_key_address    = COALESCE(NULLIF($4,''), signet_key_address),
			smart_account_address = COALESCE(NULLIF($5,''), smart_account_address),
			updated_at            = now()
		WHERE id = $1
		RETURNING id, subject, subject_kind, account_address, group_public_key,
		          email::text, display_name, avatar_url, is_staff, last_seen_at, created_at,
		          smart_account_address, signet_key_id, signet_key_address`,
		userID, groupPublicKey, keyID, keyAddress, smartAccount,
	).Scan(&u.ID, &u.Subject, &u.SubjectKind, &u.AccountAddress, &u.GroupPublicKey,
		&u.Email, &u.DisplayName, &u.AvatarURL, &u.IsStaff, &u.LastSeenAt, &u.CreatedAt,
		&u.SmartAccountAddress, &u.SignetKeyID, &u.SignetKeyAddress)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: that smart account already belongs to another user", ErrConflict)
		}
		return nil, wrap(err)
	}
	return &u, nil
}
