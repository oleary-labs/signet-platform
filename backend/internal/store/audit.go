package store

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	"github.com/oleary-labs/signet-platform/backend/internal/structs"
)

// AuditRecord is one entry to append.
type AuditRecord struct {
	OrgID      *uuid.UUID
	AppID      *uuid.UUID
	ActorID    *uuid.UUID
	ActorLabel string
	Action     string
	Target     string
	Metadata   map[string]any
	IP         string
}

// Audit appends an audit entry.
//
// Failures are logged, never returned: an audit write must not be able to fail
// the action it is describing. A missing log line is a smaller problem than a
// rejected legitimate request, and the failure is still visible in the logs.
func (s *Store) Audit(ctx context.Context, rec AuditRecord) {
	meta := json.RawMessage(`{}`)
	if len(rec.Metadata) > 0 {
		if encoded, err := json.Marshal(rec.Metadata); err == nil {
			meta = encoded
		}
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO audit_log (org_id, app_id, actor_user_id, actor_label, action, target, metadata, ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8,''))`,
		rec.OrgID, rec.AppID, rec.ActorID, rec.ActorLabel, rec.Action, rec.Target, meta, rec.IP,
	); err != nil {
		slog.Error("audit write failed", "action", rec.Action, "target", rec.Target, "error", err)
	}
}

// AuditForOrg lists an organization's recent activity.
func (s *Store) AuditForOrg(ctx context.Context, orgID uuid.UUID, limit int) ([]structs.AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.auditQuery(ctx, `
		SELECT id, app_id, actor_label, action, target, metadata, created_at
		FROM audit_log WHERE org_id=$1 ORDER BY created_at DESC LIMIT $2`, orgID, limit)
}

// AuditForApp lists one app's recent activity.
func (s *Store) AuditForApp(ctx context.Context, appID uuid.UUID, limit int) ([]structs.AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.auditQuery(ctx, `
		SELECT id, app_id, actor_label, action, target, metadata, created_at
		FROM audit_log WHERE app_id=$1 ORDER BY created_at DESC LIMIT $2`, appID, limit)
}

func (s *Store) auditQuery(ctx context.Context, query string, args ...any) ([]structs.AuditEntry, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.AuditEntry{}
	for rows.Next() {
		var e structs.AuditEntry
		if err := rows.Scan(&e.ID, &e.AppID, &e.ActorLabel, &e.Action, &e.Target, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
