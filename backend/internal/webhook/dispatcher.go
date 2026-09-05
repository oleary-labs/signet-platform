// Package webhook delivers platform events to developer-configured endpoints.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/oleary-labs/signet-platform/backend/internal/store"
)

// Dispatcher posts events to subscribed endpoints.
type Dispatcher struct {
	db   *store.Store
	http *http.Client
}

// New constructs a Dispatcher.
func New(db *store.Store) *Dispatcher {
	return &Dispatcher{db: db, http: &http.Client{Timeout: 10 * time.Second}}
}

// Envelope is the JSON body every delivery carries.
type Envelope struct {
	ID        string          `json:"id"`
	Event     string          `json:"event"`
	AppID     uuid.UUID       `json:"app_id"`
	CreatedAt time.Time       `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}

// Emit delivers an event to every subscribed endpoint, in the background.
//
// Delivery is fire-and-forget on purpose: a developer's unreachable endpoint
// must never slow down or fail the console action that produced the event.
// Every attempt is recorded, so a failing endpoint is visible in the console
// rather than silently dropped.
func (d *Dispatcher) Emit(appID uuid.UUID, event string, data any) {
	if d == nil {
		return
	}
	payload, err := json.Marshal(data)
	if err != nil {
		slog.Error("webhook payload marshal failed", "event", event, "error", err)
		return
	}
	go d.deliver(context.Background(), appID, event, payload)
}

func (d *Dispatcher) deliver(ctx context.Context, appID uuid.UUID, event string, data json.RawMessage) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	targets, err := d.db.SubscribersFor(ctx, appID, event)
	if err != nil {
		slog.Error("webhook subscriber lookup failed", "event", event, "error", err)
		return
	}
	if len(targets) == 0 {
		return
	}

	envelope := Envelope{
		ID:        uuid.NewString(),
		Event:     event,
		AppID:     appID,
		CreatedAt: time.Now().UTC(),
		Data:      data,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		slog.Error("webhook envelope marshal failed", "event", event, "error", err)
		return
	}

	for _, t := range targets {
		status, duration, derr := d.post(ctx, t, body, envelope.ID)
		errText := ""
		if derr != nil {
			errText = derr.Error()
		}
		if rerr := d.db.RecordDelivery(ctx, t.ID, event, body, status, errText, 1, int(duration.Milliseconds())); rerr != nil {
			slog.Error("webhook delivery record failed", "webhook", t.ID, "error", rerr)
		}
	}
}

func (d *Dispatcher) post(ctx context.Context, t store.WebhookTarget, body []byte, deliveryID string) (int, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, bytes.NewReader(body))
	if err != nil {
		return 0, 0, err
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "signet-platform-webhooks/1")
	req.Header.Set("Signet-Delivery", deliveryID)
	req.Header.Set("Signet-Timestamp", timestamp)
	req.Header.Set("Signet-Signature", Sign(t.Secret, timestamp, body))

	start := time.Now()
	res, err := d.http.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return 0, elapsed, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return res.StatusCode, elapsed, fmt.Errorf("endpoint returned %d", res.StatusCode)
	}
	return res.StatusCode, elapsed, nil
}

// Test delivers a single event to one endpoint, bypassing its subscription.
//
// A test has to reach the endpoint being tested. Routing it through Emit would
// mean an endpoint subscribed to anything other than the test's event silently
// received nothing, which is the opposite of what the button is for.
func (d *Dispatcher) Test(ctx context.Context, target store.WebhookTarget, appID uuid.UUID) error {
	envelope := Envelope{
		ID:        uuid.NewString(),
		Event:     "webhook.test",
		AppID:     appID,
		CreatedAt: time.Now().UTC(),
		Data: json.RawMessage(
			`{"test":true,"note":"A test delivery from the Signet platform console."}`),
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	status, duration, derr := d.post(ctx, target, body, envelope.ID)
	errText := ""
	if derr != nil {
		errText = derr.Error()
	}
	if rerr := d.db.RecordDelivery(ctx, target.ID, envelope.Event, body, status, errText, 1, int(duration.Milliseconds())); rerr != nil {
		slog.Error("test delivery record failed", "webhook", target.ID, "error", rerr)
	}
	return derr
}

// Sign produces the Signet-Signature header value.
//
// The timestamp is inside the signed material, so a captured delivery cannot
// be replayed later with a fresh timestamp — the receiver checks both.
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte{'.'})
	mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks a signature the way a receiver should, in constant time.
// Exported so the documentation can point at a real implementation rather
// than pseudocode.
func Verify(secret, timestamp string, body []byte, signature string) bool {
	return hmac.Equal([]byte(Sign(secret, timestamp, body)), []byte(signature))
}
