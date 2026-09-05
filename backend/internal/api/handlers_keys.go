package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/oleary-labs/signet-platform/backend/internal/auth"
	"github.com/oleary-labs/signet-platform/backend/internal/respond"
	"github.com/oleary-labs/signet-platform/backend/internal/store"
)

func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleViewer)
	if !ok {
		return
	}
	keys, err := s.db.Keys(r.Context(), access.AppID, store.KeyFilter{
		Curve:       r.URL.Query().Get("curve"),
		ScopeKind:   r.URL.Query().Get("scope"),
		Status:      r.URL.Query().Get("status"),
		SubjectHash: r.URL.Query().Get("subject"),
		Search:      r.URL.Query().Get("q"),
		Limit:       queryInt(r, "limit", 50),
		Offset:      queryInt(r, "offset", 0),
	})
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list keys", err)
		return
	}
	stats, err := s.db.KeyStats(r.Context(), access.AppID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not summarise keys", err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"keys": keys, "stats": stats})
}

// syncKeysRequest is the key inventory the console fetched from a node.
//
// The platform cannot fetch this itself: POST /admin/keys requires a signature
// from an authorization key the group trusts, and the platform deliberately
// holds no such key. The console makes that call with the developer's own key
// (through the node proxy) and posts the result here to cache.
type syncKeysRequest struct {
	Keys []struct {
		KeyID           string   `json:"key_id"`
		Curve           string   `json:"curve"`
		PublicKey       string   `json:"public_key"`
		EthereumAddress string   `json:"ethereum_address"`
		Address         string   `json:"address"`
		Scope           string   `json:"scope"`
		Threshold       int      `json:"threshold"`
		Parties         []string `json:"parties"`
		Status          string   `json:"status"`
	} `json:"keys"`
}

func (s *Server) handleSyncKeys(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	var req syncKeysRequest
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	records := make([]store.KeyRecord, 0, len(req.Keys))
	for _, k := range req.Keys {
		if k.KeyID == "" || k.Curve == "" {
			respond.Error(w, http.StatusBadRequest, "every key needs a key_id and a curve")
			return
		}
		switch k.Curve {
		case "frost_secp256k1", "frost_ed25519", "ecdsa_secp256k1":
		default:
			respond.Error(w, http.StatusBadRequest, "unknown curve "+k.Curve)
			return
		}
		address := k.EthereumAddress
		if address == "" {
			address = k.Address
		}
		rec := store.KeyRecord{
			KeyID:       k.KeyID,
			Curve:       k.Curve,
			PublicKey:   k.PublicKey,
			Address:     address,
			ScopeHex:    k.Scope,
			Threshold:   k.Threshold,
			Parties:     k.Parties,
			Status:      k.Status,
			SubjectHash: subjectHashFromKeyID(k.KeyID),
			ParentKeyID: parentKeyID(k.KeyID),
		}
		if err := applyScope(&rec, k.Scope); err != nil {
			respond.Error(w, http.StatusBadRequest,
				"key "+k.KeyID+": "+err.Error())
			return
		}
		records = append(records, rec)
	}

	n, err := s.db.SyncKeys(r.Context(), access.AppID, records)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not save the key inventory", err)
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "keys.synced", Target: access.AppID.String(),
		Metadata: map[string]any{"count": n}, IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusOK, map[string]any{"synced": n})
}

// subjectHashFromKeyID derives a stable per-user handle from a key ID.
//
// Auth-derived key IDs look like `oauth:<iss>:<sub>[:<suffix>]`. Hashing the
// issuer and subject means the platform can group a user's keys and count
// active wallets without ever storing the subject itself — the same privacy
// property the metering design relies on.
func subjectHashFromKeyID(keyID string) string {
	if !strings.HasPrefix(keyID, "oauth:") {
		return ""
	}
	parts := strings.Split(keyID, ":")
	if len(parts) < 3 {
		return ""
	}
	// parts[1] may itself contain colons for an https:// issuer, so the
	// subject is taken as everything up to the optional scope suffix. Rejoining
	// all but a trailing suffix keeps issuers with colons intact.
	body := strings.Join(parts[1:], ":")
	if idx := strings.LastIndex(body, ":"); idx > 0 && looksLikeScopeSuffix(body[idx+1:]) {
		body = body[:idx]
	}
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// parentKeyID returns the parent of a scoped sub-key, or "" for a root key.
func parentKeyID(keyID string) string {
	idx := strings.LastIndex(keyID, ":")
	if idx <= 0 || !looksLikeScopeSuffix(keyID[idx+1:]) {
		return ""
	}
	return keyID[:idx]
}

// looksLikeScopeSuffix reports whether a trailing segment is the 8-byte hex
// scope hash the scoped-sub-key design appends, rather than part of an issuer
// URL or subject.
func looksLikeScopeSuffix(seg string) bool {
	if len(seg) != 16 {
		return false
	}
	_, err := hex.DecodeString(seg)
	return err == nil
}

// applyScope decodes the scope bytes into the fields the console filters on.
// Layout per DESIGN-SCOPED-SUBKEYS: [1-byte scheme][scheme-specific bytes].
//
// A scope it cannot parse is an error, never a fallback to "unscoped". Showing
// a constrained key as unrestricted — or the reverse — would misstate the one
// property a developer looks at this screen to check, so a malformed scope
// fails the sync loudly instead.
func applyScope(rec *store.KeyRecord, scopeHex string) error {
	rec.ScopeKind = "unscoped"
	if strings.TrimSpace(scopeHex) == "" {
		return nil
	}
	raw, err := auth.DecodeHex(scopeHex)
	if err != nil {
		return fmt.Errorf("scope is not valid hex")
	}
	if len(raw) == 0 {
		return nil
	}
	switch raw[0] {
	case 0x00:
		return nil

	case 0x01: // EVM UserOp: entryPoint(20) | chainId(8) | sender(20)
		if len(raw) != 49 {
			return fmt.Errorf("an EVM UserOp scope must be 49 bytes, got %d", len(raw))
		}
		rec.ScopeKind = "evm_userop"
		rec.ScopeChainID = beUint64(raw[21:29])
		rec.ScopeContract = "0x" + hex.EncodeToString(raw[29:49])

	case 0x02: // Solana transaction: wallet pubkey(32)
		if len(raw) != 33 {
			return fmt.Errorf("a Solana scope must be 33 bytes, got %d", len(raw))
		}
		rec.ScopeKind = "solana_tx"

	case 0x03: // EIP-712: chainId(8) | verifyingContract(20) | typeHash(32)
		if len(raw) != 61 {
			return fmt.Errorf("an EIP-712 scope must be 61 bytes, got %d", len(raw))
		}
		rec.ScopeKind = "eip712"
		rec.ScopeChainID = beUint64(raw[1:9])
		rec.ScopeContract = "0x" + hex.EncodeToString(raw[9:29])
		rec.ScopeTypeHash = "0x" + hex.EncodeToString(raw[29:61])

	default:
		return fmt.Errorf("unknown scope scheme 0x%02x", raw[0])
	}
	return nil
}

func beUint64(b []byte) *int64 {
	if len(b) != 8 {
		return nil
	}
	var v int64
	for _, x := range b {
		v = v<<8 | int64(x)
	}
	return &v
}

func (s *Server) handleUpdateKey(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	curve := chiURLParam(r, "curve")
	keyID := chiURLParam(r, "keyID")

	var req struct {
		Label  *string `json:"label"`
		Status *string `json:"status"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Label != nil {
		if _, err := s.db.SetKeyLabel(r.Context(), access.AppID, keyID, curve, *req.Label); err != nil {
			writeStoreError(w, r, err, "could not label the key")
			return
		}
	}
	if req.Status != nil {
		switch *req.Status {
		case "enabled", "disabled":
		default:
			respond.Error(w, http.StatusBadRequest, `status must be "enabled" or "disabled"`)
			return
		}
		if _, err := s.db.SetKeyStatus(r.Context(), access.AppID, keyID, curve, *req.Status); err != nil {
			writeStoreError(w, r, err, "could not update the key")
			return
		}
		event := "key.enabled"
		if *req.Status == "disabled" {
			event = "key.disabled"
		}
		s.hooks.Emit(access.AppID, event, map[string]string{"key_id": keyID, "curve": curve})
		s.db.Audit(r.Context(), store.AuditRecord{
			OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
			Action: "key." + *req.Status, Target: keyID, IP: r.RemoteAddr,
		})
	}

	key, err := s.db.Key(r.Context(), access.AppID, keyID, curve)
	if err != nil {
		writeStoreError(w, r, err, "could not load the key")
		return
	}
	respond.JSON(w, http.StatusOK, key)
}

// ─────────────────────────── End users ───────────────────────────

func (s *Server) handleListAppUsers(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleViewer)
	if !ok {
		return
	}
	users, err := s.db.AppUsers(r.Context(), access.AppID,
		r.URL.Query().Get("q"), queryInt(r, "limit", 50), queryInt(r, "offset", 0))
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list users", err)
		return
	}
	respond.JSON(w, http.StatusOK, users)
}

func (s *Server) handleUpdateAppUser(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	userID, ok := urlUUID(w, r, "userID")
	if !ok {
		return
	}
	var req struct {
		Label string `json:"label"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.db.SetAppUserLabel(r.Context(), access.AppID, userID, req.Label); err != nil {
		writeStoreError(w, r, err, "could not label the user")
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ─────────────────────────── Delegations ───────────────────────────

func (s *Server) handleListDelegations(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleViewer)
	if !ok {
		return
	}
	delegations, err := s.db.Delegations(r.Context(), access.AppID, queryBool(r, "include_revoked"))
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list session signers", err)
		return
	}
	respond.JSON(w, http.StatusOK, delegations)
}

// handleRecordDelegation stores the metadata of a delegation the console
// minted through the nodes. Only a hash of the token is accepted — the token
// itself would let its holder sign, and the platform has no business holding
// one.
func (s *Server) handleRecordDelegation(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	var req struct {
		KeyID       string `json:"key_id"`
		ParentKeyID string `json:"parent_key_id"`
		Curve       string `json:"curve"`
		Label       string `json:"label"`
		SubjectHash string `json:"subject_hash"`
		TokenHash   string `json:"token_hash"`
		ExpiresAt   int64  `json:"expires_at"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.KeyID == "" {
		respond.Error(w, http.StatusBadRequest, "key_id is required")
		return
	}
	if req.ExpiresAt <= 0 {
		respond.Error(w, http.StatusBadRequest, "expires_at must be a unix timestamp in the future")
		return
	}
	if req.Curve == "" {
		req.Curve = "ecdsa_secp256k1"
	}
	if strings.Count(req.TokenHash, ".") > 0 {
		respond.Error(w, http.StatusBadRequest,
			"token_hash looks like a JWT — send a hash, never the delegation token itself")
		return
	}

	d, err := s.db.RecordDelegation(r.Context(), access.AppID, id.UserID, req.KeyID, req.ParentKeyID,
		req.Curve, req.Label, req.SubjectHash, req.TokenHash, time.Unix(req.ExpiresAt, 0).UTC())
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not record the session signer", err)
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "delegation.issued", Target: req.KeyID, IP: r.RemoteAddr,
	})
	s.hooks.Emit(access.AppID, "delegation.issued", d)
	respond.JSON(w, http.StatusCreated, d)
}

func (s *Server) handleRevokeDelegation(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	delegationID, ok := urlUUID(w, r, "delegationID")
	if !ok {
		return
	}
	if err := s.db.RevokeDelegation(r.Context(), access.AppID, delegationID); err != nil {
		writeStoreError(w, r, err, "could not revoke the session signer")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "delegation.revoked", Target: delegationID.String(), IP: r.RemoteAddr,
	})
	s.hooks.Emit(access.AppID, "delegation.revoked", map[string]string{"delegation_id": delegationID.String()})
	respond.JSON(w, http.StatusOK, map[string]string{
		"status": "revoked",
		"note": "Marked revoked here. A delegation token is verified by the nodes against the parent key, " +
			"so the token keeps working until you disable the sub-key on the nodes — do that too.",
	})
}

// ─────────────────────────── Policies ───────────────────────────

func (s *Server) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleViewer)
	if !ok {
		return
	}
	policies, err := s.db.Policies(r.Context(), access.AppID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list policies", err)
		return
	}
	respond.JSON(w, http.StatusOK, policies)
}

// policyEnforcement maps a policy kind to where it is actually enforced.
//
// This is the honest part of the screen. A key scope is checked independently
// by every node before it contributes a share; a smart-account rule is checked
// by the chain at execution; anything else is recorded here and enforced by
// the developer's own server. Labelling them identically would imply a
// guarantee the protocol does not currently make.
var policyEnforcement = map[string]string{
	"key_scope":   "node",
	"allowlist":   "smart_account",
	"denylist":    "smart_account",
	"spend_limit": "advisory",
	"rate_limit":  "advisory",
}

func (s *Server) handleUpsertPolicy(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	var req struct {
		Name    string          `json:"name"`
		Kind    string          `json:"kind"`
		Config  json.RawMessage `json:"config"`
		Enabled *bool           `json:"enabled"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		respond.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	enforcedBy, known := policyEnforcement[req.Kind]
	if !known {
		respond.Error(w, http.StatusBadRequest,
			"kind must be key_scope, allowlist, denylist, spend_limit, or rate_limit")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	p, err := s.db.UpsertPolicy(r.Context(), access.AppID, id.UserID,
		strings.TrimSpace(req.Name), req.Kind, enforcedBy, req.Config, enabled)
	if err != nil {
		writeStoreError(w, r, err, "could not save the policy")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "policy.saved", Target: p.Name,
		Metadata: map[string]any{"kind": p.Kind, "enforced_by": p.EnforcedBy}, IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	policyID, ok := urlUUID(w, r, "policyID")
	if !ok {
		return
	}
	if err := s.db.DeletePolicy(r.Context(), access.AppID, policyID); err != nil {
		writeStoreError(w, r, err, "could not delete the policy")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "policy.deleted", Target: policyID.String(), IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
