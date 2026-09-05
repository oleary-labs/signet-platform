package store

import (
	"context"

	"github.com/google/uuid"

	"github.com/oleary-labs/signet-platform/backend/internal/structs"
)

// RecordAsset registers an uploaded object so an org can see and reclaim what
// it holds. Called after a presigned upload completes.
func (s *Store) RecordAsset(ctx context.Context, orgID uuid.UUID, appID *uuid.UUID, uploadedBy uuid.UUID, kind, scope, objectKey, url, contentType string, size int64) (*structs.Asset, error) {
	var a structs.Asset
	err := s.pool.QueryRow(ctx, `
		INSERT INTO assets (org_id, app_id, kind, scope, object_key, url, content_type, byte_size, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (object_key) DO UPDATE SET byte_size = EXCLUDED.byte_size
		RETURNING id, app_id, kind, scope, url, content_type, byte_size, created_at`,
		orgID, appID, kind, scope, objectKey, url, contentType, size, uploadedBy,
	).Scan(&a.ID, &a.AppID, &a.Kind, &a.Scope, &a.URL, &a.ContentType, &a.ByteSize, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Assets lists an organization's uploads.
func (s *Store) Assets(ctx context.Context, orgID uuid.UUID, limit int) ([]structs.Asset, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, app_id, kind, scope, url, content_type, byte_size, created_at
		FROM assets WHERE org_id=$1 ORDER BY created_at DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.Asset{}
	for rows.Next() {
		var a structs.Asset
		if err := rows.Scan(&a.ID, &a.AppID, &a.Kind, &a.Scope, &a.URL,
			&a.ContentType, &a.ByteSize, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAsset removes an asset record and returns its storage key so the
// caller can delete the object itself.
func (s *Store) DeleteAsset(ctx context.Context, orgID, assetID uuid.UUID) (string, error) {
	var key string
	err := s.pool.QueryRow(ctx,
		`DELETE FROM assets WHERE id=$1 AND org_id=$2 RETURNING object_key`, assetID, orgID).Scan(&key)
	if err != nil {
		return "", wrap(err)
	}
	return key, nil
}
