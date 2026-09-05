// Package api wires the HTTP surface to its dependencies.
package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/oleary-labs/signet-platform/backend/internal/auth"
	"github.com/oleary-labs/signet-platform/backend/internal/chain"
	"github.com/oleary-labs/signet-platform/backend/internal/config"
	"github.com/oleary-labs/signet-platform/backend/internal/storage"
	"github.com/oleary-labs/signet-platform/backend/internal/store"
	"github.com/oleary-labs/signet-platform/backend/internal/userop"
	"github.com/oleary-labs/signet-platform/backend/internal/webhook"
)

// Server holds every dependency the handlers need.
type Server struct {
	cfg       *config.Config
	pool      *pgxpool.Pool
	db        *store.Store
	guard     *auth.Guard
	sessions  *auth.SessionIssuer
	certs     *auth.CertificateSigner
	assets    *storage.Store
	chain     *chain.Client
	hooks     *webhook.Dispatcher
	bundler   *userop.Client
	proxyHTTP *http.Client
}

// New constructs a Server.
func New(cfg *config.Config, pool *pgxpool.Pool, assets *storage.Store, chainClient *chain.Client, certs *auth.CertificateSigner) *Server {
	db := store.New(pool)
	return &Server{
		cfg:      cfg,
		pool:     pool,
		db:       db,
		guard:    auth.NewGuard(pool),
		sessions: auth.NewSessionIssuer(cfg.SessionSecret, cfg.SessionTTL),
		certs:    certs,
		assets:   assets,
		chain:    chainClient,
		hooks:    webhook.New(db),
		// A nil bundler is a valid state — the console then reports that
		// on-chain actions are unavailable instead of failing mid-request.
		bundler: userop.NewClient(cfg.BundlerURL, cfg.EntryPointAddress, cfg.BundlerAPIKey),
		// The node proxy carries threshold signing sessions, which block until
		// the protocol completes — a longer timeout than ordinary API calls.
		proxyHTTP: &http.Client{Timeout: 60 * time.Second},
	}
}

// Store exposes the query layer to background jobs in cmd/server.
func (s *Server) Store() *store.Store { return s.db }

// Router builds the HTTP handler.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(logErrors) // record every 4xx/5xx we return, with its body
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(90 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: s.cfg.CORSOrigins,
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		// X-Node-URL and X-Node-Path address the node proxies. Without them in
		// this list the browser's preflight is refused and the POST never
		// leaves — which fails as a bare network error with nothing on the
		// server to show for it, so it is worth naming why they are here.
		AllowedHeaders: []string{
			"Authorization", "Content-Type", "X-Ingest-Key", "X-App-Secret",
			"X-Node-URL", "X-Node-Path",
		},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Readiness. 503 when a dependency the server cannot work without is down.
	r.Get("/health", s.handleHealth)

	// ── Public surface ──────────────────────────────────────────────────
	// The marketplace and network status are the front door: a developer
	// evaluating operators must not have to sign in first.
	r.Get("/v1/marketplace/nodes", s.handleListNodeOperators)
	r.Get("/v1/marketplace/nodes/{address}", s.handleGetNodeOperator)
	r.Get("/v1/status", s.handleNetworkStatus)
	r.Get("/v1/config", s.handlePublicConfig)
	r.Get("/v1/webhooks/events", s.handleWebhookEventCatalogue)

	// Login. The Signet route has to reach the bootstrap nodes before a
	// session exists, so it gets its own narrow proxy: bootstrap nodes only,
	// and only the handful of paths the sign-in flow needs.
	r.Post("/v1/node/bootstrap-proxy", s.handleBootstrapProxy)
	r.Post("/v1/auth/challenge", s.handleAuthChallenge)
	r.Post("/v1/auth/verify", s.handleAuthVerify)
	r.Post("/v1/auth/logout", s.handleLogout)

	// Metering ingest, authenticated by a shared key rather than a session:
	// the caller is the node fleet, not a person.
	r.Post("/v1/ingest/usage", s.handleIngestUsage)

	// Development-only local object store, mirroring a presigned S3 PUT.
	if s.cfg.IsDev() {
		r.Put("/v1/uploads/local/*", s.handleLocalUpload)
		r.Handle("/uploads/*", http.StripPrefix("/uploads/",
			http.FileServer(http.Dir(s.cfg.LocalUploadDir))))
	}

	// Asset redirect to a short-lived presigned GET, so the bucket stays
	// private and no credential reaches the browser.
	r.Get("/v1/media/*", s.handleMedia)

	// ── Authenticated surface ───────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(s.requireSession)

		r.Get("/v1/me", s.handleGetMe)
		r.Patch("/v1/me", s.handleUpdateMe)
		r.Get("/v1/me/apps", s.handleListMyApps)
		r.Post("/v1/invites/{token}/accept", s.handleAcceptInvite)

		// Provisioning a developer's Signet key: the platform vouches for them
		// to its own group, then records what their console generated.
		r.Post("/v1/auth/node-certificate", s.handleNodeCertificate)
		r.Put("/v1/me/signet-key", s.handleRecordSignetKey)

		// The CORS proxy to signetd. The body is opaque to the platform: the
		// credential inside it belongs to the developer and is checked by the
		// node, not here.
		r.Post("/v1/node/proxy", s.handleNodeProxy)

		r.Post("/v1/uploads/presign", s.handlePresignUpload)

		// Organizations.
		r.Get("/v1/orgs", s.handleListOrgs)
		r.Post("/v1/orgs", s.handleCreateOrg)
		r.Route("/v1/orgs/{orgID}", func(r chi.Router) {
			r.Get("/", s.handleGetOrg)
			r.Patch("/", s.handleUpdateOrg)
			r.Get("/members", s.handleListMembers)
			r.Patch("/members/{userID}", s.handleUpdateMember)
			r.Delete("/members/{userID}", s.handleRemoveMember)
			r.Get("/invites", s.handleListInvites)
			r.Post("/invites", s.handleCreateInvite)
			r.Delete("/invites/{inviteID}", s.handleRevokeInvite)
			r.Get("/apps", s.handleListApps)
			r.Post("/apps", s.handleCreateApp)
			r.Get("/billing", s.handleGetBilling)
			r.Patch("/billing", s.handleUpdateBilling)
			r.Get("/invoices", s.handleListInvoices)
			r.Post("/invoices/draft", s.handleDraftInvoice)
			r.Get("/audit", s.handleOrgAudit)
			r.Get("/assets", s.handleListAssets)
			r.Delete("/assets/{assetID}", s.handleDeleteAsset)
		})

		// Apps.
		r.Route("/v1/apps/{appID}", func(r chi.Router) {
			r.Get("/", s.handleGetApp)
			r.Patch("/", s.handleUpdateApp)
			r.Delete("/", s.handleArchiveApp)

			r.Get("/environment/check", s.handleEnvironmentCheck)

			r.Get("/settings", s.handleGetSettings)
			r.Put("/settings/{section}", s.handleUpdateSettings)

			r.Get("/domains", s.handleListDomains)
			r.Post("/domains", s.handleAddDomain)
			r.Delete("/domains/{domainID}", s.handleRemoveDomain)

			r.Get("/credentials", s.handleListCredentials)
			r.Post("/credentials", s.handleCreateCredential)
			r.Delete("/credentials/{credentialID}", s.handleRevokeCredential)

			r.Get("/issuers", s.handleListIssuers)
			r.Put("/issuers", s.handleUpsertIssuer)
			r.Delete("/issuers/{issuerID}", s.handleRemoveIssuer)

			r.Get("/group", s.handleGetGroup)
			r.Post("/group/deploy", s.handleDeployGroup)
			r.Post("/group/attach", s.handleAttachGroup)
			r.Post("/group/sync", s.handleSyncGroup)
			// Membership, reshare, and auth-resolver changes are on-chain
			// calls. The console signs a UserOperation and this route submits
			// it — see handleGroupExecute for what it will and will not pay for.
			r.Post("/group/execute", s.handleGroupExecute)

			r.Get("/keys", s.handleListKeys)
			r.Post("/keys/sync", s.handleSyncKeys)
			r.Patch("/keys/{curve}/{keyID}", s.handleUpdateKey)

			r.Get("/users", s.handleListAppUsers)
			r.Patch("/users/{userID}", s.handleUpdateAppUser)

			r.Get("/delegations", s.handleListDelegations)
			r.Post("/delegations", s.handleRecordDelegation)
			r.Delete("/delegations/{delegationID}", s.handleRevokeDelegation)

			r.Get("/policies", s.handleListPolicies)
			r.Put("/policies", s.handleUpsertPolicy)
			r.Delete("/policies/{policyID}", s.handleDeletePolicy)

			r.Get("/webhooks", s.handleListWebhooks)
			r.Post("/webhooks", s.handleCreateWebhook)
			r.Patch("/webhooks/{webhookID}", s.handleUpdateWebhook)
			r.Delete("/webhooks/{webhookID}", s.handleDeleteWebhook)
			r.Get("/webhooks/{webhookID}/deliveries", s.handleListDeliveries)
			r.Post("/webhooks/{webhookID}/test", s.handleTestWebhook)

			r.Get("/usage", s.handleGetUsage)
			r.Get("/audit", s.handleAppAudit)
		})

		// Platform staff curate the marketplace; they are not org admins.
		r.Group(func(r chi.Router) {
			r.Use(s.requireStaff)
			r.Post("/v1/marketplace/nodes", s.handleUpsertNodeOperator)
			r.Patch("/v1/marketplace/nodes/{address}", s.handleUpsertNodeOperator)
		})
	})

	return r
}

// logErrors records every 4xx/5xx response — method, path, status, request id,
// and a capped slice of the body — so an error a user reports can be found in
// the logs without reproducing it.
func logErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		body := &capWriter{max: 512}
		ww.Tee(body) // capped, so a large 200 response is not buffered
		next.ServeHTTP(ww, r)
		if ww.Status() >= 400 {
			slog.Warn("error response",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"request_id", middleware.GetReqID(r.Context()),
				"body", strings.TrimSpace(body.buf.String()),
			)
		}
	})
}

// capWriter keeps only the first max bytes written.
type capWriter struct {
	buf bytes.Buffer
	max int
}

func (c *capWriter) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		if room > len(p) {
			room = len(p)
		}
		c.buf.Write(p[:room])
	}
	return len(p), nil
}
