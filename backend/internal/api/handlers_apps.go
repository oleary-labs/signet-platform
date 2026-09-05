package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/oleary-labs/signet-platform/backend/internal/auth"
	"github.com/oleary-labs/signet-platform/backend/internal/chain"
	"github.com/oleary-labs/signet-platform/backend/internal/policy"
	"github.com/oleary-labs/signet-platform/backend/internal/respond"
	"github.com/oleary-labs/signet-platform/backend/internal/store"
	"github.com/oleary-labs/signet-platform/backend/internal/structs"
	"github.com/oleary-labs/signet-platform/backend/internal/userop"
)

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := s.requireOrg(w, r, auth.RoleViewer)
	if !ok {
		return
	}
	apps, err := s.db.Apps(r.Context(), orgID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list apps", err)
		return
	}
	respond.JSON(w, http.StatusOK, apps)
}

func (s *Server) handleListMyApps(w http.ResponseWriter, r *http.Request) {
	id, ok := identity(w, r)
	if !ok {
		return
	}
	apps, err := s.db.AppsForUser(r.Context(), id.UserID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list apps", err)
		return
	}
	respond.JSON(w, http.StatusOK, apps)
}

func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	id, orgID, ok := s.requireOrg(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Environment string `json:"environment"`
		ChainID     int64  `json:"chain_id"`
		WebsiteURL  string `json:"website_url"`
		LogoURL     string `json:"logo_url"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		respond.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Environment == "" {
		req.Environment = "development"
	}
	switch req.Environment {
	case "development", "production":
	default:
		respond.Error(w, http.StatusBadRequest, "environment must be development or production")
		return
	}
	if req.ChainID == 0 {
		req.ChainID = s.cfg.ChainID
	}

	app, err := s.db.CreateApp(r.Context(), store.CreateAppInput{
		OrgID:       orgID,
		CreatedBy:   id.UserID,
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
		Environment: req.Environment,
		ChainID:     req.ChainID,
		WebsiteURL:  req.WebsiteURL,
		LogoURL:     req.LogoURL,
	})
	if err != nil {
		writeStoreError(w, r, err, "could not create the app")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &orgID, AppID: &app.ID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "app.created", Target: app.Name, IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusCreated, app)
}

func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleViewer)
	if !ok {
		return
	}
	app, err := s.db.App(r.Context(), access.AppID)
	if err != nil {
		writeStoreError(w, r, err, "could not load the app")
		return
	}
	respond.JSON(w, http.StatusOK, app)
}

func (s *Server) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Environment string `json:"environment"`
		WebsiteURL  string `json:"website_url"`
		LogoURL     string `json:"logo_url"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Environment != "" {
		switch req.Environment {
		case "development", "production":
		default:
			respond.Error(w, http.StatusBadRequest, "environment must be development or production")
			return
		}
		check, err := s.checkEnvironmentChange(r, access.AppID, req.Environment)
		if err != nil {
			respond.Fail(w, r, http.StatusInternalServerError, "could not check the environment change", err)
			return
		}
		if !check.Allowed {
			// The unmet requirements travel with the refusal, so the console can
			// show a checklist instead of a dead end.
			respond.JSON(w, http.StatusConflict, map[string]any{
				"error": "this app cannot move to " + req.Environment + " yet",
				"check": check,
			})
			return
		}
	}
	app, err := s.db.UpdateApp(r.Context(), access.AppID, store.UpdateAppInput{
		Name:        ptr(strings.TrimSpace(req.Name)),
		Description: ptr(req.Description),
		Environment: ptr(req.Environment),
		WebsiteURL:  ptr(req.WebsiteURL),
		LogoURL:     ptr(req.LogoURL),
	})
	if err != nil {
		writeStoreError(w, r, err, "could not save the app")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "app.updated", Target: app.Name, IP: r.RemoteAddr,
	})
	s.hooks.Emit(access.AppID, "app.updated", app)
	respond.JSON(w, http.StatusOK, app)
}

// handleArchiveApp archives an app.
//
// It archives the platform's record only. The signing group keeps running:
// nobody but its manager can remove its nodes, and that takes an on-chain
// timelocked removal. The response says so explicitly so the console can show
// it rather than implying the group was switched off.
func (s *Server) handleArchiveApp(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleAdmin)
	if !ok {
		return
	}
	app, err := s.db.App(r.Context(), access.AppID)
	if err != nil {
		writeStoreError(w, r, err, "could not load the app")
		return
	}
	if err := s.db.ArchiveApp(r.Context(), access.AppID); err != nil {
		writeStoreError(w, r, err, "could not archive the app")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "app.archived", Target: app.Name, IP: r.RemoteAddr,
	})

	note := "The app was archived. Its signing group is untouched and still serving requests — " +
		"to wind it down, queue a removal for each node in the group and execute it after the timelock."
	if app.GroupAddress == nil {
		note = "The app was archived. It had no signing group deployed, so nothing on-chain was affected."
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "archived", "note": note})
}

// ─────────────────────────── Settings ───────────────────────────

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleViewer)
	if !ok {
		return
	}
	settings, err := s.db.Settings(r.Context(), access.AppID)
	if err != nil {
		writeStoreError(w, r, err, "could not load settings")
		return
	}
	respond.JSON(w, http.StatusOK, settings)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	section := chiURLParam(r, "section")

	// The section document is developer-defined, so it is decoded as raw JSON
	// rather than a fixed struct — but it must still be a JSON object, not a
	// bare scalar that would break every reader downstream.
	var doc json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&doc); err != nil {
		respond.Error(w, http.StatusBadRequest, "settings body must be a JSON document")
		return
	}
	trimmed := strings.TrimSpace(string(doc))
	if !strings.HasPrefix(trimmed, "{") {
		respond.Error(w, http.StatusBadRequest, "settings body must be a JSON object")
		return
	}

	settings, err := s.db.UpdateSettings(r.Context(), access.AppID, section, doc)
	if err != nil {
		if strings.HasPrefix(err.Error(), "unknown settings section") {
			respond.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		writeStoreError(w, r, err, "could not save settings")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "app.settings_updated", Target: section, IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusOK, settings)
}

// ─────────────────────────── Domains ───────────────────────────

func (s *Server) handleListDomains(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleViewer)
	if !ok {
		return
	}
	domains, err := s.db.Domains(r.Context(), access.AppID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list domains", err)
		return
	}
	respond.JSON(w, http.StatusOK, domains)
}

func (s *Server) handleAddDomain(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	var req struct {
		Origin string `json:"origin"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	origin, err := normalizeOrigin(req.Origin)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	domain, err := s.db.AddDomain(r.Context(), access.AppID, origin)
	if err != nil {
		writeStoreError(w, r, err, "could not add the domain")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "app.domain_added", Target: origin, IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusCreated, domain)
}

func (s *Server) handleRemoveDomain(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	domainID, ok := urlUUID(w, r, "domainID")
	if !ok {
		return
	}
	if err := s.db.RemoveDomain(r.Context(), access.AppID, domainID); err != nil {
		writeStoreError(w, r, err, "could not remove the domain")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "app.domain_removed", Target: domainID.String(), IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// normalizeOrigin validates an origin the way a browser reports one: scheme
// and host, nothing else. Storing a full URL here would silently never match
// the Origin header the SDK actually sends, so anything carrying a path,
// query, fragment, or credentials is rejected rather than trimmed.
func normalizeOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errBadOrigin
	}
	// A bare host is the common way people type this, so assume https rather
	// than rejecting it.
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	// A single trailing slash is what a copy-paste from the address bar leaves
	// behind; drop exactly that, never more.
	raw = strings.TrimSuffix(raw, "/")

	u, err := url.Parse(raw)
	if err != nil {
		return "", errBadOrigin
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errBadOrigin
	}
	if u.Host == "" || u.Hostname() == "" {
		return "", errBadOrigin
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", errBadOrigin
	}
	return u.Scheme + "://" + u.Host, nil
}

var errBadOrigin = &originError{}

type originError struct{}

func (*originError) Error() string {
	return "origin must be a scheme and host, such as https://app.example.com — no path, query, or fragment"
}

// ─────────────────────────── Credentials ───────────────────────────

func (s *Server) handleListCredentials(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	creds, err := s.db.Credentials(r.Context(), access.AppID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list credentials", err)
		return
	}
	respond.JSON(w, http.StatusOK, creds)
}

// handleCreateCredential mints an app secret, or records an authorization key.
//
// For an auth key the request carries the *public* half only. The private key
// is generated in the developer's browser and shown once there; sending it
// here would defeat the point of the group holding the trust anchor.
func (s *Server) handleCreateCredential(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	var req struct {
		Kind      string         `json:"kind"`
		Label     string         `json:"label"`
		PublicKey string         `json:"public_key"`
		UserOp    *userop.Packed `json:"user_op"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	switch req.Kind {
	case "app_secret", "":
		cred, err := s.db.CreateAppSecret(r.Context(), access.AppID, id.UserID, req.Label)
		if err != nil {
			respond.Fail(w, r, http.StatusInternalServerError, "could not create the secret", err)
			return
		}
		s.db.Audit(r.Context(), store.AuditRecord{
			OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
			Action: "credential.secret_created", Target: cred.ID.String(), IP: r.RemoteAddr,
		})
		respond.JSON(w, http.StatusCreated, cred)

	case "auth_key":
		raw, err := auth.DecodeHex(req.PublicKey)
		if err != nil || (len(raw) != 33 && len(raw) != 34 && len(raw) != 65) {
			respond.Error(w, http.StatusBadRequest,
				"public_key must be a hex secp256k1 key (33 bytes compressed, 34 scheme-prefixed, or 65 uncompressed)")
			return
		}
		keyHash, err := chain.AuthKeyHash(req.PublicKey)
		if err != nil {
			respond.Error(w, http.StatusBadRequest, "could not hash the public key")
			return
		}
		// An authorization key that only this database knows about is a key
		// the nodes will reject. Where a group exists, register it on-chain
		// first and record it as active; only a group-less app can legitimately
		// hold one pending, to be included when the group is created.
		app, err := s.db.App(r.Context(), access.AppID)
		if err != nil {
			writeStoreError(w, r, err, "could not load the app")
			return
		}
		status, txHash := "pending", ""
		if app.GroupAddress != nil {
			sender, _, err := s.smartWallet(r, id)
			if err != nil {
				respond.Error(w, http.StatusBadRequest, err.Error())
				return
			}
			receipt, _, ok := s.submitUserOpDecoded(w, r, req.UserOp, userop.Intent{
				Action:          "adding an authorization key",
				Dest:            *app.GroupAddress,
				Selectors:       map[string]string{"addAuthKey": selAddAuthKey},
				RefusePaymaster: app.Environment != "development",
			}, sender, func(d *userop.Decoded) error {
				got, ok := d.BytesArg(0)
				if !ok {
					return fmt.Errorf("addAuthKey is missing its public key")
				}
				if !bytes.Equal(got, raw) {
					return fmt.Errorf("the operation registers key 0x%s, but this request records a different one",
						hex.EncodeToString(got))
				}
				return nil
			})
			if !ok {
				return
			}
			status, txHash = "active", receipt.TransactionHash
		}

		cred, err := s.db.RecordAuthKey(r.Context(), access.AppID, id.UserID, req.Label,
			strings.ToLower(req.PublicKey), keyHash, status)
		if err != nil {
			respond.Fail(w, r, http.StatusInternalServerError, "could not record the authorization key", err)
			return
		}
		s.db.Audit(r.Context(), store.AuditRecord{
			OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
			Action: "credential.auth_key_recorded", Target: keyHash,
			Metadata: map[string]any{"transaction_hash": txHash, "status": status}, IP: r.RemoteAddr,
		})
		s.hooks.Emit(access.AppID, "auth_key.added", cred)
		respond.JSON(w, http.StatusCreated, cred)

	default:
		respond.Error(w, http.StatusBadRequest, `kind must be "app_secret" or "auth_key"`)
	}
}

func (s *Server) handleRevokeCredential(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	credID, ok := urlUUID(w, r, "credentialID")
	if !ok {
		return
	}
	var req struct {
		UserOp *userop.Packed `json:"user_op"`
	}
	// An app secret is revoked in this database alone, so a bodyless DELETE is
	// the normal case.
	_ = respond.Decode(r, &req)

	creds, err := s.db.Credentials(r.Context(), access.AppID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list credentials", err)
		return
	}
	var target *structs.Credential
	for i := range creds {
		if creds[i].ID == credID {
			target = &creds[i]
			break
		}
	}
	if target == nil {
		respond.Error(w, http.StatusNotFound, "no such credential")
		return
	}

	// Revoking an authorization key here without removing it on-chain leaves
	// the nodes still accepting it — the console would show it gone while it
	// kept working. So where the key is actually registered, the on-chain
	// removal is part of revoking it, not a follow-up step for the developer.
	txHash := ""
	if target.Kind == "auth_key" && target.OnchainStatus == "active" {
		app, err := s.db.App(r.Context(), access.AppID)
		if err != nil {
			writeStoreError(w, r, err, "could not load the app")
			return
		}
		if app.GroupAddress != nil {
			if target.KeyHash == nil {
				respond.Error(w, http.StatusConflict,
					"this key has no recorded hash, so it cannot be removed on-chain from here")
				return
			}
			sender, _, err := s.smartWallet(r, id)
			if err != nil {
				respond.Error(w, http.StatusBadRequest, err.Error())
				return
			}
			receipt, _, ok := s.submitUserOpDecoded(w, r, req.UserOp, userop.Intent{
				Action:          "removing an authorization key",
				Dest:            *app.GroupAddress,
				Selectors:       map[string]string{"removeAuthKey": selRemoveAuthKey},
				RefusePaymaster: app.Environment != "development",
			}, sender, func(d *userop.Decoded) error {
				word, ok := d.Word(0)
				if !ok {
					return fmt.Errorf("removeAuthKey is missing its key hash")
				}
				got := "0x" + hex.EncodeToString(word[:])
				if !strings.EqualFold(got, *target.KeyHash) {
					return fmt.Errorf("the operation removes key %s, but this request revokes %s",
						got, *target.KeyHash)
				}
				return nil
			})
			if !ok {
				return
			}
			txHash = receipt.TransactionHash
		}
	}

	if err := s.db.RevokeCredential(r.Context(), access.AppID, credID); err != nil {
		writeStoreError(w, r, err, "could not revoke the credential")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "credential.revoked", Target: credID.String(),
		Metadata: map[string]any{"kind": target.Kind, "transaction_hash": txHash}, IP: r.RemoteAddr,
	})
	s.hooks.Emit(access.AppID, "auth_key.revoked", map[string]string{"credential_id": credID.String()})
	respond.JSON(w, http.StatusOK, map[string]any{
		"status":           "revoked",
		"transaction_hash": txHash,
	})
}

// ─────────────────────────── Issuers ───────────────────────────

func (s *Server) handleListIssuers(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleViewer)
	if !ok {
		return
	}
	issuers, err := s.db.Issuers(r.Context(), access.AppID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list login methods", err)
		return
	}
	respond.JSON(w, http.StatusOK, issuers)
}

func (s *Server) handleUpsertIssuer(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	var req struct {
		Issuer    string         `json:"issuer"`
		ClientIDs []string       `json:"client_ids"`
		Provider  string         `json:"provider"`
		Label     string         `json:"label"`
		UserOp    *userop.Packed `json:"user_op"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Issuer = strings.TrimSpace(req.Issuer)
	if req.Issuer == "" {
		respond.Error(w, http.StatusBadRequest, "issuer is required")
		return
	}
	// A client-ID list containing an empty string matches nothing at all,
	// which reads as "allow none" rather than the "allow any" the developer
	// almost certainly meant. Strip the blanks rather than storing that trap.
	cleaned := make([]string, 0, len(req.ClientIDs))
	for _, c := range req.ClientIDs {
		if c = strings.TrimSpace(c); c != "" {
			cleaned = append(cleaned, c)
		}
	}
	if req.Provider == "" {
		req.Provider = guessProvider(req.Issuer)
	}

	// A login method only means anything once the group's nodes trust it, so
	// once a group exists this route writes the chain first and the mirror
	// second. Recording it either way is what used to leave the console
	// claiming an issuer was configured while every node rejected it.
	txHash := ""
	app, err := s.db.App(r.Context(), access.AppID)
	if err != nil {
		writeStoreError(w, r, err, "could not load the app")
		return
	}

	// Editing a label or a provider icon changes nothing the operators can
	// see, and neither does re-recording an issuer that createGroup already
	// wrote. Charging a transaction for either would be a fee on bookkeeping.
	needsChain := app.GroupAddress != nil
	if needsChain {
		existing, err := s.db.Issuers(r.Context(), access.AppID)
		if err != nil {
			respond.Fail(w, r, http.StatusInternalServerError, "could not list login methods", err)
			return
		}
		for _, e := range existing {
			if e.Issuer == req.Issuer && e.OnchainStatus == "active" && sameClientIDs(e.ClientIDs, cleaned) {
				needsChain = false
				break
			}
		}
	}

	if needsChain {
		sender, _, err := s.smartWallet(r, id)
		if err != nil {
			respond.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		receipt, ok := s.submitUserOp(w, r, req.UserOp, userop.Intent{
			Action:          "adding a login method",
			Dest:            *app.GroupAddress,
			Selectors:       map[string]string{"addIssuer": selAddIssuer},
			RefusePaymaster: app.Environment != "development",
		}, sender)
		if !ok {
			return
		}
		txHash = receipt.TransactionHash
	}

	issuer, err := s.db.UpsertIssuer(r.Context(), access.AppID, req.Issuer,
		chain.IssuerHash(req.Issuer), cleaned, req.Provider, req.Label)
	if err != nil {
		writeStoreError(w, r, err, "could not save the login method")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "issuer.saved", Target: req.Issuer,
		Metadata: map[string]any{"client_ids": len(cleaned), "transaction_hash": txHash}, IP: r.RemoteAddr,
	})
	s.hooks.Emit(access.AppID, "issuer.added", issuer)
	respond.JSON(w, http.StatusOK, map[string]any{
		"issuer":           issuer,
		"transaction_hash": txHash,
	})
}

func (s *Server) handleRemoveIssuer(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	issuerID, ok := urlUUID(w, r, "issuerID")
	if !ok {
		return
	}
	var req struct {
		UserOp *userop.Packed `json:"user_op"`
	}
	// DELETE with no body is fine for an app that has no group yet.
	_ = respond.Decode(r, &req)

	app, err := s.db.App(r.Context(), access.AppID)
	if err != nil {
		writeStoreError(w, r, err, "could not load the app")
		return
	}

	txHash := ""
	if app.GroupAddress != nil {
		issuers, err := s.db.Issuers(r.Context(), access.AppID)
		if err != nil {
			respond.Fail(w, r, http.StatusInternalServerError, "could not list login methods", err)
			return
		}
		var target *structs.Issuer
		for i := range issuers {
			if issuers[i].ID == issuerID {
				target = &issuers[i]
				break
			}
		}
		if target == nil {
			respond.Error(w, http.StatusNotFound, "no such login method")
			return
		}

		sender, _, err := s.smartWallet(r, id)
		if err != nil {
			respond.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		receipt, decoded, ok := s.submitUserOpDecoded(w, r, req.UserOp, userop.Intent{
			Action:          "removing a login method",
			Dest:            *app.GroupAddress,
			Selectors:       map[string]string{"removeIssuer": selRemoveIssuer},
			RefusePaymaster: app.Environment != "development",
		}, sender, func(d *userop.Decoded) error {
			// removeIssuer takes the issuer hash. Checking it keeps the two
			// records in step: without this, deleting issuer A here while the
			// operation removed issuer B on-chain leaves both wrong.
			word, ok := d.Word(0)
			if !ok {
				return fmt.Errorf("removeIssuer is missing its issuer hash")
			}
			got := "0x" + hex.EncodeToString(word[:])
			if target.IssuerHash == nil || !strings.EqualFold(got, *target.IssuerHash) {
				return fmt.Errorf("the operation removes issuer %s, but this request removes %s",
					got, target.Issuer)
			}
			return nil
		})
		if !ok {
			return
		}
		_ = decoded
		txHash = receipt.TransactionHash
	}

	if err := s.db.RemoveIssuer(r.Context(), access.AppID, issuerID); err != nil {
		writeStoreError(w, r, err, "could not remove the login method")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "issuer.removed", Target: issuerID.String(),
		Metadata: map[string]any{"transaction_hash": txHash}, IP: r.RemoteAddr,
	})
	s.hooks.Emit(access.AppID, "issuer.removed", map[string]string{"issuer_id": issuerID.String()})
	respond.JSON(w, http.StatusOK, map[string]any{
		"status":           "removed",
		"transaction_hash": txHash,
	})
}

// sameClientIDs compares two allow-lists as sets. Order is not meaningful on
// chain, so a reordered list is not a change worth a transaction.
func sameClientIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		seen[v]--
		if seen[v] < 0 {
			return false
		}
	}
	return true
}

// guessProvider picks the console icon for a well-known issuer.
func guessProvider(issuer string) string {
	switch {
	case strings.Contains(issuer, "accounts.google.com"):
		return "google"
	case strings.Contains(issuer, "appleid.apple.com"):
		return "apple"
	case strings.Contains(issuer, "github"):
		return "github"
	case strings.Contains(issuer, "auth0"):
		return "auth0"
	case strings.Contains(issuer, "okta"):
		return "okta"
	case strings.Contains(issuer, "microsoftonline"):
		return "microsoft"
	default:
		return "custom"
	}
}

func (s *Server) handleAppAudit(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	entries, err := s.db.AuditForApp(r.Context(), access.AppID, queryInt(r, "limit", 100))
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not load the audit log", err)
		return
	}
	respond.JSON(w, http.StatusOK, entries)
}

// checkEnvironmentChange evaluates the platform's rules for moving an app to a
// new environment.
//
// It re-reads the group from the chain first where it can. Promotion is gated
// on who is actually in the group, and a stale cache would let an app be
// promoted on a composition it no longer has.
func (s *Server) checkEnvironmentChange(r *http.Request, appID uuid.UUID, to string) (*policy.EnvironmentCheck, error) {
	app, err := s.db.App(r.Context(), appID)
	if err != nil {
		return nil, err
	}
	if to == "production" && app.GroupAddress != nil && s.chain.Enabled() {
		// A sync failure is not fatal: fall through to the cached membership
		// rather than blocking a promotion on a flaky RPC endpoint. The
		// composition shown to the user carries its own synced_at.
		_ = s.syncGroup(r.Context(), app)
	}

	plan, err := s.db.OrgPlan(r.Context(), appID)
	if err != nil {
		return nil, err
	}
	total, firstParty, external, err := s.db.GroupComposition(r.Context(), appID, policy.FirstPartyCategory)
	if err != nil {
		return nil, err
	}

	check := policy.CheckEnvironment(app.Environment, to, plan, policy.GroupComposition{
		ActiveTotal:      total,
		ActiveFirstParty: firstParty,
		ActiveExternal:   external,
	}, app.GroupAddress != nil)
	return &check, nil
}

// handleEnvironmentCheck answers "what would it take to move this app to that
// environment" without attempting the move.
//
// The console reads it to render the promotion checklist, so a developer can
// see what is missing before they try, rather than discovering it through a
// refusal.
func (s *Server) handleEnvironmentCheck(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleViewer)
	if !ok {
		return
	}
	to := r.URL.Query().Get("to")
	if to == "" {
		to = "production"
	}
	switch to {
	case "development", "production":
	default:
		respond.Error(w, http.StatusBadRequest, "to must be development or production")
		return
	}

	check, err := s.checkEnvironmentChange(r, access.AppID, to)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not check the environment change", err)
		return
	}
	respond.JSON(w, http.StatusOK, check)
}
