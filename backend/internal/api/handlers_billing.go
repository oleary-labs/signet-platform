package api

import (
	"net/http"
	"time"

	"github.com/oleary-labs/signet-platform/backend/internal/auth"
	"github.com/oleary-labs/signet-platform/backend/internal/respond"
)

// paymentsEnabled reports whether the platform can actually move money.
//
// It is a constant false today, and every billing response says so. The
// schema, the ledger, and the invoice draft are in place so switching payments
// on is a change to settlement, not a redesign — but until then the console
// must not imply an org is being charged.
const paymentsEnabled = false

func (s *Server) handleGetBilling(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := s.requireOrg(w, r, auth.RoleAdmin)
	if !ok {
		return
	}
	account, err := s.db.BillingAccount(r.Context(), orgID, paymentsEnabled)
	if err != nil {
		writeStoreError(w, r, err, "could not load billing")
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"account": account,
		"note": "Payments are not switched on. Usage is metered and the projection below is what it " +
			"would cost at the published per-active-wallet rate; nothing is being charged.",
	})
}

func (s *Server) handleUpdateBilling(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := s.requireOrg(w, r, auth.RoleOwner)
	if !ok {
		return
	}
	var req struct {
		ContractAddress  string `json:"contract_address"`
		ChainID          *int64 `json:"chain_id"`
		LowBalanceMicros *int64 `json:"low_balance_micros"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ContractAddress != "" {
		normalized, err := auth.NormalizeAddress(req.ContractAddress)
		if err != nil {
			respond.Error(w, http.StatusBadRequest, "invalid billing contract address")
			return
		}
		req.ContractAddress = normalized
	}
	if err := s.db.UpdateBillingSettings(r.Context(), orgID, req.ContractAddress, req.ChainID, req.LowBalanceMicros); err != nil {
		writeStoreError(w, r, err, "could not save billing settings")
		return
	}
	account, err := s.db.BillingAccount(r.Context(), orgID, paymentsEnabled)
	if err != nil {
		writeStoreError(w, r, err, "could not load billing")
		return
	}
	respond.JSON(w, http.StatusOK, account)
}

func (s *Server) handleListInvoices(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := s.requireOrg(w, r, auth.RoleAdmin)
	if !ok {
		return
	}
	invoices, err := s.db.Invoices(r.Context(), orgID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list invoices", err)
		return
	}
	respond.JSON(w, http.StatusOK, invoices)
}

// handleDraftInvoice computes a draft for a period from metered usage. It
// never issues or charges — that boundary stays until payments are switched on.
func (s *Server) handleDraftInvoice(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := s.requireOrg(w, r, auth.RoleAdmin)
	if !ok {
		return
	}
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, -1)

	var req struct {
		PeriodStart string `json:"period_start"`
		PeriodEnd   string `json:"period_end"`
	}
	if err := respond.Decode(r, &req); err == nil {
		if req.PeriodStart != "" {
			if t, perr := time.Parse("2006-01-02", req.PeriodStart); perr == nil {
				start = t
			}
		}
		if req.PeriodEnd != "" {
			if t, perr := time.Parse("2006-01-02", req.PeriodEnd); perr == nil {
				end = t
			}
		}
	}
	if end.Before(start) {
		respond.Error(w, http.StatusBadRequest, "period_end must be on or after period_start")
		return
	}

	invoice, err := s.db.DraftInvoice(r.Context(), orgID, start, end)
	if err != nil {
		writeStoreError(w, r, err, "could not draft the invoice")
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"invoice": invoice,
		"note":    "Draft only — computed from metered usage. Nothing has been charged.",
	})
}
