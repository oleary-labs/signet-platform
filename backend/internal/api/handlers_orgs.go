package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/oleary-labs/signet-platform/backend/internal/auth"
	"github.com/oleary-labs/signet-platform/backend/internal/respond"
	"github.com/oleary-labs/signet-platform/backend/internal/store"
)

// inviteTTL is how long an invitation link stays redeemable.
const inviteTTL = 7 * 24 * time.Hour

func (s *Server) handleListOrgs(w http.ResponseWriter, r *http.Request) {
	id, ok := identity(w, r)
	if !ok {
		return
	}
	orgs, err := s.db.OrgsForUser(r.Context(), id.UserID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list organizations", err)
		return
	}
	respond.JSON(w, http.StatusOK, orgs)
}

func (s *Server) handleCreateOrg(w http.ResponseWriter, r *http.Request) {
	id, ok := identity(w, r)
	if !ok {
		return
	}
	var req struct {
		Name         string `json:"name"`
		BillingEmail string `json:"billing_email"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		respond.Error(w, http.StatusBadRequest, "name is required")
		return
	}

	org, err := s.db.CreateOrg(r.Context(), id.UserID, strings.TrimSpace(req.Name), req.BillingEmail)
	if err != nil {
		writeStoreError(w, r, err, "could not create the organization")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &org.ID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "org.created", Target: org.Name, IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusCreated, org)
}

func (s *Server) handleGetOrg(w http.ResponseWriter, r *http.Request) {
	id, orgID, ok := s.requireOrg(w, r, auth.RoleViewer)
	if !ok {
		return
	}
	org, err := s.db.Org(r.Context(), orgID)
	if err != nil {
		writeStoreError(w, r, err, "could not load the organization")
		return
	}
	if role, err := s.guard.OrgRole(r.Context(), id.UserID, orgID); err == nil {
		org.Role = string(role)
	}
	respond.JSON(w, http.StatusOK, org)
}

func (s *Server) handleUpdateOrg(w http.ResponseWriter, r *http.Request) {
	id, orgID, ok := s.requireOrg(w, r, auth.RoleAdmin)
	if !ok {
		return
	}
	var req struct {
		Name         string `json:"name"`
		BillingEmail string `json:"billing_email"`
		LogoURL      string `json:"logo_url"`
		WebsiteURL   string `json:"website_url"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	org, err := s.db.UpdateOrg(r.Context(), orgID, req.Name, req.BillingEmail, req.LogoURL, req.WebsiteURL)
	if err != nil {
		writeStoreError(w, r, err, "could not save the organization")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &orgID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "org.updated", Target: org.Name, IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusOK, org)
}

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := s.requireOrg(w, r, auth.RoleViewer)
	if !ok {
		return
	}
	members, err := s.db.Members(r.Context(), orgID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list members", err)
		return
	}
	respond.JSON(w, http.StatusOK, members)
}

func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request) {
	id, orgID, ok := s.requireOrg(w, r, auth.RoleAdmin)
	if !ok {
		return
	}
	memberID, ok := urlUUID(w, r, "userID")
	if !ok {
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !auth.Role(req.Role).Valid() {
		respond.Error(w, http.StatusBadRequest, "role must be owner, admin, developer, or viewer")
		return
	}
	// Only an owner can mint another owner; an admin promoting themselves
	// would otherwise be a one-click privilege escalation.
	if req.Role == string(auth.RoleOwner) {
		if _, err := s.guard.RequireOrg(r.Context(), id.UserID, orgID, auth.RoleOwner); err != nil {
			respond.Error(w, http.StatusForbidden, "only an owner can grant the owner role")
			return
		}
	}
	if err := s.db.SetMemberRole(r.Context(), orgID, memberID, req.Role); err != nil {
		if err == store.ErrNotFound {
			respond.Error(w, http.StatusNotFound, "that person is not a member")
			return
		}
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &orgID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "org.member_role_changed", Target: memberID.String(),
		Metadata: map[string]any{"role": req.Role}, IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	id, orgID, ok := s.requireOrg(w, r, auth.RoleAdmin)
	if !ok {
		return
	}
	memberID, ok := urlUUID(w, r, "userID")
	if !ok {
		return
	}
	if err := s.db.RemoveMember(r.Context(), orgID, memberID); err != nil {
		if err == store.ErrNotFound {
			respond.Error(w, http.StatusNotFound, "that person is not a member")
			return
		}
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &orgID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "org.member_removed", Target: memberID.String(), IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (s *Server) handleListInvites(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := s.requireOrg(w, r, auth.RoleAdmin)
	if !ok {
		return
	}
	invites, err := s.db.Invites(r.Context(), orgID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list invitations", err)
		return
	}
	respond.JSON(w, http.StatusOK, invites)
}

// handleCreateInvite issues an invitation link.
//
// The raw token is returned exactly once, in this response, and the console
// hands it to the admin to send. The platform does not send email itself, so
// there is no silent failure mode where an invitation is "sent" but never
// arrives.
func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	id, orgID, ok := s.requireOrg(w, r, auth.RoleAdmin)
	if !ok {
		return
	}
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		respond.Error(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	if req.Role == "" {
		req.Role = string(auth.RoleDeveloper)
	}
	if req.Role == string(auth.RoleOwner) || !auth.Role(req.Role).Valid() {
		respond.Error(w, http.StatusBadRequest, "role must be admin, developer, or viewer")
		return
	}

	invite, err := s.db.CreateInvite(r.Context(), orgID, id.UserID, req.Email, req.Role, inviteTTL)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not create the invitation", err)
		return
	}
	invite.AcceptURL = strings.TrimRight(s.cfg.PublicWebURL, "/") + "/invite/" + invite.Token

	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &orgID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "org.invite_created", Target: req.Email,
		Metadata: map[string]any{"role": req.Role}, IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusCreated, invite)
}

func (s *Server) handleRevokeInvite(w http.ResponseWriter, r *http.Request) {
	id, orgID, ok := s.requireOrg(w, r, auth.RoleAdmin)
	if !ok {
		return
	}
	inviteID, ok := urlUUID(w, r, "inviteID")
	if !ok {
		return
	}
	if err := s.db.RevokeInvite(r.Context(), orgID, inviteID); err != nil {
		writeStoreError(w, r, err, "could not revoke the invitation")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &orgID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "org.invite_revoked", Target: inviteID.String(), IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	id, ok := identity(w, r)
	if !ok {
		return
	}
	token := chiURLParam(r, "token")
	if token == "" {
		respond.Error(w, http.StatusBadRequest, "missing invitation token")
		return
	}
	org, err := s.db.AcceptInvite(r.Context(), id.UserID, token)
	if err != nil {
		if err == store.ErrNotFound {
			respond.Error(w, http.StatusNotFound, "that invitation is expired, already used, or revoked")
			return
		}
		respond.Fail(w, r, http.StatusInternalServerError, "could not accept the invitation", err)
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &org.ID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "org.invite_accepted", Target: org.Name, IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusOK, org)
}

func (s *Server) handleOrgAudit(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := s.requireOrg(w, r, auth.RoleAdmin)
	if !ok {
		return
	}
	entries, err := s.db.AuditForOrg(r.Context(), orgID, queryInt(r, "limit", 100))
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not load the audit log", err)
		return
	}
	respond.JSON(w, http.StatusOK, entries)
}
