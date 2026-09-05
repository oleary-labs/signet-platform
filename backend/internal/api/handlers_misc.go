package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/oleary-labs/signet-platform/backend/internal/auth"
	"github.com/oleary-labs/signet-platform/backend/internal/health"
	"github.com/oleary-labs/signet-platform/backend/internal/nodeapi"
	"github.com/oleary-labs/signet-platform/backend/internal/respond"
	"github.com/oleary-labs/signet-platform/backend/internal/storage"
	"github.com/oleary-labs/signet-platform/backend/internal/store"
)

// handleHealth is the readiness probe.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	var pinger health.AssetPinger
	if s.assets != nil {
		pinger = s.assets.Ping
	}
	st := health.Check(r.Context(), s.pool, pinger)
	if !st.OK {
		respond.JSON(w, http.StatusServiceUnavailable, st)
		return
	}
	respond.JSON(w, http.StatusOK, st)
}

func (s *Server) handleWebhookEventCatalogue(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusOK, map[string]any{"events": store.WebhookEvents})
}

// ─────────────────────────── Node proxy ───────────────────────────

// handleNodeProxy forwards a console request to a signetd node.
//
// signetd sets no CORS headers, so a browser cannot reach it directly. The
// forwarded body is opaque here on purpose: the session signature or auth-key
// certificate inside it was produced by the developer's own key and is checked
// by the node. The platform adds no authority of its own to the request — it
// only decides *which* node the request may reach, and which paths exist.
func (s *Server) handleNodeProxy(w http.ResponseWriter, r *http.Request) {
	nodeURL := r.Header.Get("x-node-url")
	path := r.Header.Get("x-node-path")
	method := r.Header.Get("x-node-method")
	if method == "" {
		method = http.MethodPost
	}
	if nodeURL == "" || path == "" {
		respond.Error(w, http.StatusBadRequest, "x-node-url and x-node-path headers are required")
		return
	}
	if !s.nodeURLAllowed(r, nodeURL) {
		respond.Error(w, http.StatusForbidden,
			"that node is not in the operator directory or your bootstrap set")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "could not read the request body")
		return
	}

	res, err := nodeapi.Proxy(r.Context(), s.proxyHTTP, nodeapi.ProxyRequest{
		NodeURL: nodeURL,
		Path:    path,
		Method:  method,
		Body:    body,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not proxyable") {
			respond.Error(w, http.StatusForbidden, err.Error())
			return
		}
		respond.Fail(w, r, http.StatusBadGateway, "the node did not respond", err)
		return
	}

	w.Header().Set("Content-Type", res.ContentType)
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(res.Body)
}

// nodeURLAllowed restricts the proxy to nodes the platform actually knows
// about. Without this the endpoint would be a general-purpose request forwarder
// that any signed-in developer could point at an internal address.
func (s *Server) nodeURLAllowed(r *http.Request, nodeURL string) bool {
	target := strings.TrimRight(strings.ToLower(nodeURL), "/")
	for _, n := range s.cfg.BootstrapNodes {
		if strings.TrimRight(strings.ToLower(n), "/") == target {
			return true
		}
	}
	operators, err := s.db.NodeOperators(r.Context(), store.NodeOperatorFilter{})
	if err != nil {
		return false
	}
	for _, o := range operators {
		if o.APIURL != nil && strings.TrimRight(strings.ToLower(*o.APIURL), "/") == target {
			return true
		}
	}
	return false
}

// ─────────────────────────── Uploads ───────────────────────────

func (s *Server) handlePresignUpload(w http.ResponseWriter, r *http.Request) {
	id, ok := identity(w, r)
	if !ok {
		return
	}
	var req struct {
		OrgID       string `json:"org_id"`
		AppID       string `json:"app_id"`
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		Scope       string `json:"scope"`
		ByteSize    int64  `json:"byte_size"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	orgID, err := uuid.Parse(req.OrgID)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "org_id is required")
		return
	}
	if _, err := s.guard.RequireOrg(r.Context(), id.UserID, orgID, auth.RoleDeveloper); err != nil {
		writeAuthzError(w, r, err)
		return
	}
	var appID *uuid.UUID
	if req.AppID != "" {
		parsed, err := uuid.Parse(req.AppID)
		if err != nil {
			respond.Error(w, http.StatusBadRequest, "invalid app_id")
			return
		}
		if _, err := s.guard.RequireApp(r.Context(), id.UserID, parsed, auth.RoleDeveloper); err != nil {
			writeAuthzError(w, r, err)
			return
		}
		appID = &parsed
	}

	// Uploads are rendered back into the console and into developers' own
	// login modals, so the accepted types exclude anything script-bearing.
	ext, err := storage.ValidateContentType(req.ContentType)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	const maxUpload = 8 << 20
	if req.ByteSize > maxUpload {
		respond.Error(w, http.StatusBadRequest, "uploads are limited to 8 MB")
		return
	}

	scope := req.Scope
	if scope == "" {
		scope = "misc"
	}
	key, err := storage.NewObjectKey(orgID.String()+"/"+scope, ext)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not prepare the upload", err)
		return
	}

	var uploadURL, publicURL string
	if s.assets != nil {
		uploadURL, publicURL, err = s.assets.PresignUpload(r.Context(), key, req.ContentType)
		if err != nil {
			respond.Fail(w, r, http.StatusInternalServerError, "could not prepare the upload", err)
			return
		}
	} else {
		// Development: an unauthenticated PUT to an unguessable key, mirroring
		// the shape of a presigned S3 upload so the client code is identical.
		base := strings.TrimRight(s.publicAPIURL(r), "/")
		uploadURL = base + "/v1/uploads/local/" + key
		publicURL = base + "/uploads/" + key
	}

	asset, err := s.db.RecordAsset(r.Context(), orgID, appID, id.UserID, "image", scope, key, publicURL, req.ContentType, req.ByteSize)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not record the upload", err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"upload_url": uploadURL,
		"public_url": publicURL,
		"object_key": key,
		"asset":      asset,
	})
}

// handleLocalUpload is the development stand-in for a presigned S3 PUT.
func (s *Server) handleLocalUpload(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/v1/uploads/local/")
	// The key is attacker-controlled text, so it is cleaned and re-checked to
	// be inside the upload directory before anything is written.
	dest := filepath.Join(s.cfg.LocalUploadDir, filepath.Clean("/"+key))
	root, err := filepath.Abs(s.cfg.LocalUploadDir)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "upload failed", err)
		return
	}
	abs, err := filepath.Abs(dest)
	if err != nil || !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		respond.Error(w, http.StatusBadRequest, "invalid upload key")
		return
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "upload failed", err)
		return
	}
	f, err := os.Create(abs)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "upload failed", err)
		return
	}
	defer f.Close()
	if _, err := io.Copy(f, io.LimitReader(r.Body, 8<<20)); err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "upload failed", err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "stored"})
}

// handleMedia redirects to a short-lived presigned GET, so the bucket stays
// private and no credential reaches the browser.
func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/v1/media/")
	if key == "" {
		respond.Error(w, http.StatusBadRequest, "missing object key")
		return
	}
	if s.assets == nil {
		http.Redirect(w, r, strings.TrimRight(s.publicAPIURL(r), "/")+"/uploads/"+key, http.StatusFound)
		return
	}
	url, err := s.assets.PresignGet(r.Context(), key)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not load that asset", err)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Server) handleListAssets(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := s.requireOrg(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	assets, err := s.db.Assets(r.Context(), orgID, queryInt(r, "limit", 100))
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list assets", err)
		return
	}
	respond.JSON(w, http.StatusOK, assets)
}

func (s *Server) handleDeleteAsset(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := s.requireOrg(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	assetID, ok := urlUUID(w, r, "assetID")
	if !ok {
		return
	}
	key, err := s.db.DeleteAsset(r.Context(), orgID, assetID)
	if err != nil {
		writeStoreError(w, r, err, "could not delete the asset")
		return
	}
	if s.assets != nil {
		// The record is already gone; a failed object delete leaves an orphan
		// in the bucket rather than a broken reference in the console, so it
		// is logged and not surfaced as a failure to the user.
		if err := s.assets.Delete(r.Context(), key); err != nil {
			respond.Fail(w, r, http.StatusOK, "deleted, but the stored object could not be removed", err)
			return
		}
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// publicAPIURL reconstructs this server's externally visible base URL.
func (s *Server) publicAPIURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// ─────────────────────────── Metering ingest ───────────────────────────

// handleIngestUsage accepts metered protocol events from the node fleet.
//
// It is authenticated by a shared key rather than a session because the caller
// is infrastructure, not a person. Events carry a hashed subject, never a raw
// OAuth subject, so the platform can count active wallets — the billing
// unit — without learning who an app's users are.
func (s *Server) handleIngestUsage(w http.ResponseWriter, r *http.Request) {
	if s.cfg.IngestKey == "" {
		respond.Error(w, http.StatusNotFound, "usage ingest is not enabled on this server")
		return
	}
	if subtleCompare(r.Header.Get("X-Ingest-Key"), s.cfg.IngestKey) != 1 {
		respond.Error(w, http.StatusUnauthorized, "invalid ingest key")
		return
	}

	var req struct {
		Events []struct {
			GroupAddress string `json:"group_address"`
			AppID        string `json:"app_id"`
			SubjectHash  string `json:"subject_hash"`
			Kind         string `json:"kind"`
			Curve        string `json:"curve"`
			KeyID        string `json:"key_id"`
			NodeAddress  string `json:"node_address"`
			LatencyMS    int    `json:"latency_ms"`
			OK           *bool  `json:"ok"`
			ErrorCode    string `json:"error_code"`
			OccurredAt   int64  `json:"occurred_at"`
			Issuer       string `json:"issuer"`
		} `json:"events"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Events) == 0 {
		respond.JSON(w, http.StatusOK, map[string]int{"accepted": 0})
		return
	}
	const maxBatch = 1000
	if len(req.Events) > maxBatch {
		respond.Error(w, http.StatusBadRequest, "at most 1000 events per request")
		return
	}

	events := make([]store.UsageEvent, 0, len(req.Events))
	skipped := 0
	for _, e := range req.Events {
		appID, ok := s.resolveIngestApp(r, e.AppID, e.GroupAddress)
		if !ok {
			// An event for a group the platform does not know about is
			// counted and dropped, not an error: the node fleet serves groups
			// that were never created through this platform.
			skipped++
			continue
		}
		switch e.Kind {
		case "auth", "keygen", "sign", "delegate", "reshare":
		default:
			skipped++
			continue
		}
		okFlag := true
		if e.OK != nil {
			okFlag = *e.OK
		}
		at := time.Now()
		if e.OccurredAt > 0 {
			at = time.Unix(e.OccurredAt, 0)
		}
		events = append(events, store.UsageEvent{
			AppID:       appID,
			SubjectHash: e.SubjectHash,
			Kind:        e.Kind,
			Curve:       e.Curve,
			KeyID:       e.KeyID,
			NodeAddress: e.NodeAddress,
			LatencyMS:   e.LatencyMS,
			OK:          okFlag,
			ErrorCode:   e.ErrorCode,
			OccurredAt:  at,
		})
		if e.SubjectHash != "" {
			_ = s.db.TouchAppUser(r.Context(), appID, e.SubjectHash, e.Issuer)
		}
	}

	if err := s.db.RecordUsage(r.Context(), events); err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not record usage", err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]int{"accepted": len(events), "skipped": skipped})
}

// resolveIngestApp maps an event to an app by explicit id or by group address.
func (s *Server) resolveIngestApp(r *http.Request, appID, groupAddress string) (uuid.UUID, bool) {
	if appID != "" {
		if parsed, err := uuid.Parse(appID); err == nil {
			return parsed, true
		}
		return uuid.Nil, false
	}
	if groupAddress == "" {
		return uuid.Nil, false
	}
	app, err := s.db.AppByGroup(r.Context(), groupAddress)
	if err != nil {
		return uuid.Nil, false
	}
	return app.ID, true
}

// subtleCompare is a constant-time string comparison, so a wrong ingest key
// cannot be recovered by timing the rejection.
func subtleCompare(a, b string) int {
	if len(a) != len(b) {
		return 0
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	if diff == 0 {
		return 1
	}
	return 0
}

// ─────────────────────────── Usage ───────────────────────────

func (s *Server) handleGetUsage(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleViewer)
	if !ok {
		return
	}
	to := queryDate(r, "to", time.Now().UTC())
	from := queryDate(r, "from", to.AddDate(0, 0, -29))
	if from.After(to) {
		respond.Error(w, http.StatusBadRequest, "from must be on or before to")
		return
	}
	if to.Sub(from) > 400*24*time.Hour {
		respond.Error(w, http.StatusBadRequest, "the range is limited to 400 days")
		return
	}
	summary, err := s.db.Usage(r.Context(), access.AppID, from, to)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not load usage", err)
		return
	}
	respond.JSON(w, http.StatusOK, summary)
}

// ─────────────────────────── Webhooks ───────────────────────────

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	hooks, err := s.db.Webhooks(r.Context(), access.AppID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list webhooks", err)
		return
	}
	respond.JSON(w, http.StatusOK, hooks)
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	var req struct {
		URL         string   `json:"url"`
		Events      []string `json:"events"`
		Description string   `json:"description"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !strings.HasPrefix(req.URL, "https://") && !strings.HasPrefix(req.URL, "http://") {
		respond.Error(w, http.StatusBadRequest, "url must be an http or https endpoint")
		return
	}
	for _, e := range req.Events {
		if !store.ValidEvent(e) {
			respond.Error(w, http.StatusBadRequest, "unknown event "+e)
			return
		}
	}

	hook, err := s.db.CreateWebhook(r.Context(), access.AppID, id.UserID, req.URL, req.Events, req.Description)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not create the webhook", err)
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "webhook.created", Target: req.URL, IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusCreated, hook)
}

func (s *Server) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	hookID, ok := urlUUID(w, r, "webhookID")
	if !ok {
		return
	}
	var req struct {
		URL         *string  `json:"url"`
		Events      []string `json:"events"`
		Enabled     *bool    `json:"enabled"`
		Description *string  `json:"description"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	for _, e := range req.Events {
		if !store.ValidEvent(e) {
			respond.Error(w, http.StatusBadRequest, "unknown event "+e)
			return
		}
	}
	hook, err := s.db.UpdateWebhook(r.Context(), access.AppID, hookID, req.URL, req.Events, req.Enabled, req.Description)
	if err != nil {
		writeStoreError(w, r, err, "could not update the webhook")
		return
	}
	respond.JSON(w, http.StatusOK, hook)
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	hookID, ok := urlUUID(w, r, "webhookID")
	if !ok {
		return
	}
	if err := s.db.DeleteWebhook(r.Context(), access.AppID, hookID); err != nil {
		writeStoreError(w, r, err, "could not delete the webhook")
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleListDeliveries(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	hookID, ok := urlUUID(w, r, "webhookID")
	if !ok {
		return
	}
	deliveries, err := s.db.Deliveries(r.Context(), access.AppID, hookID, queryInt(r, "limit", 50))
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not load deliveries", err)
		return
	}
	respond.JSON(w, http.StatusOK, deliveries)
}

func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	hookID, ok := urlUUID(w, r, "webhookID")
	if !ok {
		return
	}
	target, err := s.db.WebhookTargetByID(r.Context(), access.AppID, hookID)
	if err != nil {
		writeStoreError(w, r, err, "could not load the webhook")
		return
	}

	// Delivered synchronously so the response can report what actually
	// happened — a queued test that quietly failed is no more useful than not
	// having tested at all.
	if err := s.hooks.Test(r.Context(), *target, access.AppID); err != nil {
		respond.JSON(w, http.StatusOK, map[string]any{
			"delivered": false,
			"error":     err.Error(),
			"note":      "The endpoint did not accept the delivery. The attempt is recorded below.",
		})
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"delivered": true,
		"note":      "The endpoint accepted the test delivery.",
	})
}

// bootstrapProxyPaths is the exact set of node paths the unauthenticated
// bootstrap proxy will forward. Sign-in needs /v1/auth, /v1/keygen, and
// /v1/sign; nothing else belongs on a surface reachable without a session.
var bootstrapProxyPaths = map[string]bool{
	"/v1/health": true,
	"/v1/info":   true,
	"/v1/auth":   true,
	"/v1/keygen": true,
	"/v1/sign":   true,
}

// handleBootstrapProxy forwards a sign-in request to a bootstrap node.
//
// It is unauthenticated because it is what a developer uses to *become*
// authenticated: the console proves control of a Signet account by having the
// bootstrap group sign a challenge, which requires reaching those nodes first.
// The blast radius is held down by two constraints — the destination must be
// one of this deployment's configured bootstrap nodes, and the path must be
// one of the five above. Everything inside the body is verified by the node
// against the developer's own session key, not by the platform.
func (s *Server) handleBootstrapProxy(w http.ResponseWriter, r *http.Request) {
	nodeURL := r.Header.Get("x-node-url")
	path := r.Header.Get("x-node-path")
	if nodeURL == "" || path == "" {
		respond.Error(w, http.StatusBadRequest, "x-node-url and x-node-path headers are required")
		return
	}
	if !bootstrapProxyPaths[path] {
		respond.Error(w, http.StatusForbidden, "that path is not reachable before sign-in")
		return
	}

	target := strings.TrimRight(strings.ToLower(nodeURL), "/")
	allowed := false
	for _, n := range s.cfg.BootstrapNodes {
		if strings.TrimRight(strings.ToLower(n), "/") == target {
			allowed = true
			break
		}
	}
	if !allowed {
		respond.Error(w, http.StatusForbidden, "that node is not part of this deployment's bootstrap group")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "could not read the request body")
		return
	}

	res, err := nodeapi.Proxy(r.Context(), s.proxyHTTP, nodeapi.ProxyRequest{
		NodeURL: nodeURL,
		Path:    path,
		Method:  http.MethodPost,
		Body:    body,
	})
	if err != nil {
		respond.Fail(w, r, http.StatusBadGateway, "the bootstrap node did not respond", err)
		return
	}
	w.Header().Set("Content-Type", res.ContentType)
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(res.Body)
}
