package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/oleary-labs/signet-platform/backend/internal/auth"
	"github.com/oleary-labs/signet-platform/backend/internal/nodeapi"
	"github.com/oleary-labs/signet-platform/backend/internal/respond"
	"github.com/oleary-labs/signet-platform/backend/internal/store"
)

// handleListNodeOperators serves the public marketplace directory.
//
// It is deliberately unauthenticated: a developer evaluating who they would
// trust with a share of their users' keys should not have to create an account
// to see the operator set first.
func (s *Server) handleListNodeOperators(w http.ResponseWriter, r *http.Request) {
	operators, err := s.db.NodeOperators(r.Context(), store.NodeOperatorFilter{
		Category:   r.URL.Query().Get("category"),
		Region:     r.URL.Query().Get("region"),
		OnlyOpen:   queryBool(r, "open"),
		OnlyOnline: queryBool(r, "online"),
		Search:     r.URL.Query().Get("q"),
	})
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not list node operators", err)
		return
	}
	respond.JSON(w, http.StatusOK, operators)
}

func (s *Server) handleGetNodeOperator(w http.ResponseWriter, r *http.Request) {
	operator, err := s.db.NodeOperator(r.Context(), chiURLParam(r, "address"))
	if err != nil {
		writeStoreError(w, r, err, "could not load the node operator")
		return
	}
	respond.JSON(w, http.StatusOK, operator)
}

// handleUpsertNodeOperator lets platform staff curate a listing. The on-chain
// half of the row is refreshed by the indexer, so a staff edit here can never
// misstate whether a node is registered or open.
func (s *Server) handleUpsertNodeOperator(w http.ResponseWriter, r *http.Request) {
	id, ok := identity(w, r)
	if !ok {
		return
	}
	var req struct {
		Address      string `json:"address"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		WebsiteURL   string `json:"website_url"`
		LogoURL      string `json:"logo_url"`
		APIURL       string `json:"api_url"`
		Region       string `json:"region"`
		Jurisdiction string `json:"jurisdiction"`
		Category     string `json:"category"`
		ContactEmail string `json:"contact_email"`
		Verified     *bool  `json:"verified"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Address == "" {
		req.Address = chiURLParam(r, "address")
	}
	address, err := auth.NormalizeAddress(req.Address)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid node address")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		respond.Error(w, http.StatusBadRequest, "name is required")
		return
	}

	operator, err := s.db.UpsertNodeOperator(r.Context(), store.UpsertNodeOperatorInput{
		Address:      address,
		Name:         strings.TrimSpace(req.Name),
		Description:  req.Description,
		WebsiteURL:   req.WebsiteURL,
		LogoURL:      req.LogoURL,
		APIURL:       strings.TrimRight(req.APIURL, "/"),
		Region:       req.Region,
		Jurisdiction: req.Jurisdiction,
		Category:     req.Category,
		ContactEmail: req.ContactEmail,
		Verified:     req.Verified,
	})
	if err != nil {
		writeStoreError(w, r, err, "could not save the node operator")
		return
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "marketplace.operator_saved", Target: address, IP: r.RemoteAddr,
	})
	respond.JSON(w, http.StatusOK, operator)
}

// NetworkStatus is the public status page's payload.
type NetworkStatus struct {
	ChainID         int64     `json:"chain_id"`
	FactoryAddress  string    `json:"factory_address"`
	ChainReachable  bool      `json:"chain_reachable"`
	ChainError      string    `json:"chain_error,omitempty"`
	RegisteredNodes int       `json:"registered_nodes"`
	Groups          int       `json:"groups"`
	OperatorsListed int       `json:"operators_listed"`
	OperatorsOnline int       `json:"operators_online"`
	CheckedAt       time.Time `json:"checked_at"`
}

func (s *Server) handleNetworkStatus(w http.ResponseWriter, r *http.Request) {
	status := NetworkStatus{
		ChainID:        s.cfg.ChainID,
		FactoryAddress: s.cfg.FactoryAddress,
		CheckedAt:      time.Now().UTC(),
	}

	operators, err := s.db.NodeOperators(r.Context(), store.NodeOperatorFilter{})
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not read network status", err)
		return
	}
	status.OperatorsListed = len(operators)
	for _, o := range operators {
		if o.Health != nil && o.Health.Online {
			status.OperatorsOnline++
		}
	}

	// The chain is reported as a dependency, not a precondition: an unreachable
	// RPC endpoint is exactly the condition this page exists to show.
	if s.chain.Enabled() {
		if nodes, err := s.chain.RegisteredNodes(r.Context()); err != nil {
			status.ChainError = err.Error()
		} else {
			status.ChainReachable = true
			status.RegisteredNodes = len(nodes)
			if groups, err := s.chain.Groups(r.Context()); err == nil {
				status.Groups = len(groups)
			}
		}
	} else {
		status.ChainError = "chain reads are not configured on this server"
	}
	respond.JSON(w, http.StatusOK, status)
}

// ProbeNodes samples every listed operator's liveness. Called on a timer so
// the marketplace shows current health without a visitor's page load having to
// wait on a dozen cross-region HTTP calls.
func (s *Server) ProbeNodes(ctx context.Context) error {
	targets, err := s.db.ProbeTargets(ctx)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}

	// Probes run concurrently but bounded, so a large operator set cannot open
	// hundreds of sockets at once.
	const parallelism = 8
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup

	for address, apiURL := range targets {
		wg.Add(1)
		go func(address, apiURL string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			ok, latency, peers, probeErr := nodeapi.New(apiURL).Probe(probeCtx)
			msg := ""
			if probeErr != nil {
				msg = probeErr.Error()
			}
			if err := s.db.RecordHealthSample(ctx, address, ok, int(latency.Milliseconds()), peers, msg); err != nil {
				slog.Error("health sample write failed", "node", address, "error", err)
			}
		}(address, apiURL)
	}
	wg.Wait()
	return nil
}

// SyncNodeRegistry refreshes the marketplace's on-chain half from the factory.
func (s *Server) SyncNodeRegistry(ctx context.Context) error {
	if !s.chain.Enabled() {
		return nil
	}
	nodes, err := s.chain.RegisteredNodes(ctx)
	if err != nil {
		return err
	}
	var firstErr error
	for _, addr := range nodes {
		info, err := s.chain.Node(ctx, addr)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		groups, err := s.chain.NodeGroups(ctx, addr)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		registeredAt := time.Unix(info.RegisteredAt, 0).UTC()
		if err := s.db.SyncNodeRegistry(ctx, info.Address, info.IsOpen, registeredAt, info.Operator, len(groups)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
