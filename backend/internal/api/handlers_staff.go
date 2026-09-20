package api

import (
	"net/http"

	"github.com/oleary-labs/signet-platform/backend/internal/respond"
)

// The staff overview: the one part of this API that reads across tenants.
//
// It exists because sponsorship is open. The platform pays to deploy groups for
// anyone who signs in, which is the posture we want while courting developers —
// but "watch for abuse and react" is only a strategy if someone can watch.
// Without this the first signal would be the paymaster balance dropping.
//
// It is read-only by construction. Staff curate the operator marketplace and
// can now see who is using the platform; they cannot reach into an
// organization, open its apps, or act on anyone's behalf. Being able to observe
// a tenant is already a privilege worth keeping narrow.
func (s *Server) handleStaffOverview(w http.ResponseWriter, r *http.Request) {
	totals, err := s.db.StaffTotals(r.Context())
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not read platform totals", err)
		return
	}
	users, err := s.db.StaffUsers(r.Context(), queryInt(r, "users", 50))
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list accounts", err)
		return
	}
	apps, err := s.db.StaffApps(r.Context(), queryInt(r, "apps", 50))
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list apps", err)
		return
	}

	respond.JSON(w, http.StatusOK, map[string]any{
		"totals": totals,
		"users":  users,
		"apps":   apps,
		// What the deploy route is actually enforcing, so the page can say
		// whether sponsorship is open and what the ceiling is rather than
		// leaving staff to infer it from the environment.
		"sponsorship": map[string]any{
			"enabled":           s.cfg.SponsorGroupCreation,
			"invitation_only":   len(s.cfg.SponsoredSubjects) > 0,
			"invited_count":     len(s.cfg.SponsoredSubjects),
			"max_per_subject":   s.cfg.MaxSponsoredGroupsPerSubject,
			"paymaster_address": s.cfg.PaymasterAddress,
		},
	})
}
