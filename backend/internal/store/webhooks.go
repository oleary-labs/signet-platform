package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/oleary-labs/signet-platform/backend/internal/structs"
)

// WebhookEvents is the catalogue the console offers. Keeping it explicit stops
// a typo'd event name from being subscribed to and then never firing.
var WebhookEvents = []string{
	"app.deployed",
	"app.updated",
	"group.node_invited",
	"group.node_joined",
	"group.removal_queued",
	"group.removal_executed",
	"group.reshare_requested",
	"issuer.added",
	"issuer.removed",
	"auth_key.added",
	"auth_key.revoked",
	"key.created",
	"key.disabled",
	"key.enabled",
	"delegation.issued",
	"delegation.revoked",
	"user.first_seen",
	"usage.threshold_reached",
	"billing.low_balance",
}

// ValidEvent reports whether name is a known event.
func ValidEvent(name string) bool {
	for _, e := range WebhookEvents {
		if e == name {
			return true
		}
	}
	return false
}

// Webhooks lists an app's endpoints. The signing secret is never returned
// after creation, so a compromised console session cannot read it back and
// forge deliveries.
func (s *Store) Webhooks(ctx context.Context, appID uuid.UUID) ([]structs.Webhook, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, url, events, enabled, description, created_at
		FROM webhooks WHERE app_id=$1 ORDER BY created_at`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.Webhook{}
	for rows.Next() {
		var w structs.Webhook
		if err := rows.Scan(&w.ID, &w.URL, &w.Events, &w.Enabled, &w.Description, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// CreateWebhook registers an endpoint and returns its signing secret once.
func (s *Store) CreateWebhook(ctx context.Context, appID, createdBy uuid.UUID, url string, events []string, description string) (*structs.Webhook, error) {
	secret, err := RandomToken(32)
	if err != nil {
		return nil, err
	}
	full := "whsec_" + secret
	if events == nil {
		events = []string{}
	}

	var w structs.Webhook
	err = s.pool.QueryRow(ctx, `
		INSERT INTO webhooks (app_id, url, secret, events, description, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, url, events, enabled, description, created_at`,
		appID, url, full, events, description, createdBy,
	).Scan(&w.ID, &w.URL, &w.Events, &w.Enabled, &w.Description, &w.CreatedAt)
	if err != nil {
		return nil, err
	}
	w.Secret = full
	return &w, nil
}

// UpdateWebhook edits an endpoint's subscription and enabled state.
func (s *Store) UpdateWebhook(ctx context.Context, appID, webhookID uuid.UUID, url *string, events []string, enabled *bool, description *string) (*structs.Webhook, error) {
	var w structs.Webhook
	err := s.pool.QueryRow(ctx, `
		UPDATE webhooks SET
			url         = COALESCE($3, url),
			events      = COALESCE($4, events),
			enabled     = COALESCE($5, enabled),
			description = COALESCE($6, description)
		WHERE id=$1 AND app_id=$2
		RETURNING id, url, events, enabled, description, created_at`,
		webhookID, appID, url, events, enabled, description,
	).Scan(&w.ID, &w.URL, &w.Events, &w.Enabled, &w.Description, &w.CreatedAt)
	if err != nil {
		return nil, wrap(err)
	}
	return &w, nil
}

// DeleteWebhook removes an endpoint.
func (s *Store) DeleteWebhook(ctx context.Context, appID, webhookID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM webhooks WHERE id=$1 AND app_id=$2`, webhookID, appID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// WebhookTarget is one endpoint the dispatcher should deliver to.
type WebhookTarget struct {
	ID     uuid.UUID
	URL    string
	Secret string
}

// SubscribersFor returns the enabled endpoints subscribed to an event. An
// endpoint with an empty event list receives everything, which is the least
// surprising reading of "I didn't narrow it".
func (s *Store) SubscribersFor(ctx context.Context, appID uuid.UUID, event string) ([]WebhookTarget, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, url, secret FROM webhooks
		WHERE app_id=$1 AND enabled AND (cardinality(events) = 0 OR $2 = ANY(events))`,
		appID, event)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []WebhookTarget{}
	for rows.Next() {
		var t WebhookTarget
		if err := rows.Scan(&t.ID, &t.URL, &t.Secret); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// WebhookTargetByID returns one endpoint and its secret, for a test delivery.
func (s *Store) WebhookTargetByID(ctx context.Context, appID, webhookID uuid.UUID) (*WebhookTarget, error) {
	var t WebhookTarget
	err := s.pool.QueryRow(ctx,
		`SELECT id, url, secret FROM webhooks WHERE id=$1 AND app_id=$2`, webhookID, appID,
	).Scan(&t.ID, &t.URL, &t.Secret)
	if err != nil {
		return nil, wrap(err)
	}
	return &t, nil
}

// RecordDelivery stores the outcome of one delivery attempt.
func (s *Store) RecordDelivery(ctx context.Context, webhookID uuid.UUID, event string, payload json.RawMessage, statusCode int, deliveryErr string, attempt, durationMS int) error {
	var code *int
	if statusCode > 0 {
		code = &statusCode
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO webhook_deliveries (webhook_id, event, payload, status_code, error, attempt, duration_ms)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), $6, NULLIF($7,0))`,
		webhookID, event, payload, code, deliveryErr, attempt, durationMS)
	return err
}

// Deliveries lists recent attempts for one endpoint.
func (s *Store) Deliveries(ctx context.Context, appID, webhookID uuid.UUID, limit int) ([]structs.WebhookDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.event, d.payload, d.status_code, d.error, d.attempt, d.duration_ms, d.created_at
		FROM webhook_deliveries d
		JOIN webhooks w ON w.id = d.webhook_id AND w.app_id = $1
		WHERE d.webhook_id = $2
		ORDER BY d.created_at DESC
		LIMIT $3`, appID, webhookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.WebhookDelivery{}
	for rows.Next() {
		var d structs.WebhookDelivery
		if err := rows.Scan(&d.ID, &d.Event, &d.Payload, &d.StatusCode, &d.Error,
			&d.Attempt, &d.DurationMS, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// PruneDeliveries drops delivery history past the retention window.
func (s *Store) PruneDeliveries(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM webhook_deliveries WHERE created_at < now() - interval '30 days'`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
