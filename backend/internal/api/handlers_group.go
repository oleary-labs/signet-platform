package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/oleary-labs/signet-platform/backend/internal/auth"
	"github.com/oleary-labs/signet-platform/backend/internal/chain"
	"github.com/oleary-labs/signet-platform/backend/internal/policy"
	"github.com/oleary-labs/signet-platform/backend/internal/respond"
	"github.com/oleary-labs/signet-platform/backend/internal/store"
	"github.com/oleary-labs/signet-platform/backend/internal/structs"
	"github.com/oleary-labs/signet-platform/backend/internal/userop"
)

// GroupView is what the group screen renders: the app's cached membership plus
// a live read of the contract when the RPC endpoint is reachable.
type GroupView struct {
	App     *structs.App        `json:"app"`
	Nodes   []structs.GroupNode `json:"nodes"`
	Onchain *chain.GroupState   `json:"onchain"`
	// SyncError explains why Onchain is absent, instead of the screen quietly
	// showing stale cached data as though it were current.
	SyncError string `json:"sync_error,omitempty"`
}

func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleViewer)
	if !ok {
		return
	}
	app, err := s.db.App(r.Context(), access.AppID)
	if err != nil {
		writeStoreError(w, r, err, "could not load the app")
		return
	}
	nodes, err := s.db.GroupNodes(r.Context(), access.AppID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not load group membership", err)
		return
	}

	view := GroupView{App: app, Nodes: nodes}
	if app.GroupAddress != nil && s.chain.Enabled() {
		state, err := s.chain.GroupState(r.Context(), *app.GroupAddress)
		if err != nil {
			view.SyncError = err.Error()
		} else {
			view.Onchain = state
		}
	} else if app.GroupAddress == nil {
		view.SyncError = "no signing group is attached to this app yet"
	} else {
		view.SyncError = "chain reads are not configured on this server"
	}
	respond.JSON(w, http.StatusOK, view)
}

// handleAttachGroup binds a deployed signing group to an app.
//
// The group is deployed by the developer's own UserOperation — the platform
// never writes to the chain. Before accepting the binding the server reads the
// contract and checks the caller's own account is its manager, so one org
// cannot claim another's group by pasting its address.
func (s *Server) handleAttachGroup(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	var req struct {
		GroupAddress   string `json:"group_address"`
		GroupPublicKey string `json:"group_public_key"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	groupAddress, err := auth.NormalizeAddress(req.GroupAddress)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid group address")
		return
	}

	threshold, nodeCount := 0, 0
	manageNote := ""
	if s.chain.Enabled() {
		state, err := s.chain.GroupState(r.Context(), groupAddress)
		if err != nil {
			respond.Error(w, http.StatusBadRequest,
				"could not read that group from the chain — check the address and that the RPC endpoint is reachable: "+err.Error())
			return
		}
		user, err := s.db.User(r.Context(), id.UserID)
		if err != nil {
			writeStoreError(w, r, err, "could not load your account")
			return
		}
		if !managerMatches(state.Manager, user.SmartAccountAddress, user.AccountAddress) {
			respond.Error(w, http.StatusForbidden,
				"that group is managed by "+state.Manager+", which is not an account you control")
			return
		}
		// The console sends its transactions from the smart wallet. If the
		// group answers to something else, attaching still works — the
		// developer owns it — but every button on the group screen would
		// revert, so say so now rather than at the first click.
		if !managerMatches(state.Manager, user.SmartAccountAddress) {
			manageNote = "This group is managed by " + state.Manager +
				", not your smart wallet, so the console cannot change it for you. Manage it from that account, or transfer the manager role to your smart wallet."
		}
		threshold = int(state.Threshold)
		nodeCount = len(state.ActiveNodes)
	}

	// A development group runs on O'Leary Labs operators only. The check reads
	// the chain directly rather than the cache, because at this point the app
	// has no cached membership at all — this is the moment it acquires one.
	if current, err := s.db.App(r.Context(), access.AppID); err == nil &&
		current.Environment == "development" && s.chain.Enabled() {
		if msg, err := s.developmentOperatorRefusal(r, groupAddress); err != nil {
			respond.Fail(w, r, http.StatusInternalServerError, "could not check the group's operators", err)
			return
		} else if msg != "" {
			respond.Error(w, http.StatusConflict, msg)
			return
		}
	}

	app, err := s.db.AttachGroup(r.Context(), access.AppID, groupAddress, req.GroupPublicKey, threshold, nodeCount)
	if err != nil {
		writeStoreError(w, r, err, "could not attach the signing group")
		return
	}
	if s.chain.Enabled() {
		if err := s.syncGroup(r.Context(), app); err != nil {
			// The binding is already recorded and correct; a failed first sync
			// only means the membership cache fills on the next refresh.
			respond.JSON(w, http.StatusOK, map[string]any{
				"app": app, "sync_error": err.Error(), "manage_note": manageNote,
			})
			return
		}
		app, _ = s.db.App(r.Context(), access.AppID)
	}

	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "app.group_attached", Target: groupAddress, IP: r.RemoteAddr,
	})
	s.hooks.Emit(access.AppID, "app.deployed", app)
	respond.JSON(w, http.StatusOK, map[string]any{"app": app, "manage_note": manageNote})
}

// managerMatches reports whether an on-chain manager is one of the addresses
// this developer controls.
//
// There are two. Groups created through the console are managed by the
// developer's smart wallet, because that is what called createGroup. A group
// created elsewhere — from their own wallet, or a script — is managed by
// whatever sent that transaction, usually their EOA. Both are theirs, so both
// are accepted; what differs is whether the console can act on the group
// afterwards, which is what managedBySmartWallet is for.
func managerMatches(manager string, addresses ...*string) bool {
	for _, a := range addresses {
		if a != nil && *a != "" && strings.EqualFold(manager, *a) {
			return true
		}
	}
	return false
}

func (s *Server) handleSyncGroup(w http.ResponseWriter, r *http.Request) {
	_, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	app, err := s.db.App(r.Context(), access.AppID)
	if err != nil {
		writeStoreError(w, r, err, "could not load the app")
		return
	}
	if app.GroupAddress == nil {
		respond.Error(w, http.StatusBadRequest, "this app has no signing group attached yet")
		return
	}
	if !s.chain.Enabled() {
		respond.Error(w, http.StatusServiceUnavailable, "chain reads are not configured on this server")
		return
	}
	if err := s.syncGroup(r.Context(), app); err != nil {
		respond.Fail(w, r, http.StatusBadGateway, "could not read the group from the chain", err)
		return
	}
	nodes, err := s.db.GroupNodes(r.Context(), access.AppID)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not load group membership", err)
		return
	}
	app, _ = s.db.App(r.Context(), access.AppID)
	respond.JSON(w, http.StatusOK, GroupView{App: app, Nodes: nodes})
}

// syncGroup refreshes every cached projection of one group's on-chain state:
// the app summary, the membership snapshot, and the issuer and auth-key
// mirrors. Doing them together is what keeps the console's several screens
// from disagreeing with each other.
func (s *Server) syncGroup(ctx context.Context, app *structs.App) error {
	if app.GroupAddress == nil {
		return nil
	}
	state, err := s.chain.GroupState(ctx, *app.GroupAddress)
	if err != nil {
		return err
	}

	nodes := make([]store.GroupNodeState, 0, len(state.ActiveNodes)+len(state.PendingNodes))
	removing := map[string]*chain.RemovalRequest{}
	for i := range state.PendingRemovals {
		rr := state.PendingRemovals[i]
		removing[strings.ToLower(rr.Node)] = &rr
	}
	for _, n := range state.ActiveNodes {
		entry := store.GroupNodeState{Address: n, Status: "active"}
		if rr, ok := removing[strings.ToLower(n)]; ok {
			entry.Status = "removing"
			at := time.Unix(rr.ExecuteAfter, 0).UTC()
			entry.ExecuteAfter = &at
			entry.Initiator = rr.Initiator
		}
		nodes = append(nodes, entry)
	}
	for _, n := range state.PendingNodes {
		nodes = append(nodes, store.GroupNodeState{Address: n, Status: "pending"})
	}
	if err := s.db.ReplaceGroupNodes(ctx, app.ID, nodes); err != nil {
		return err
	}

	if err := s.db.SyncGroupState(ctx, app.ID, int(state.Threshold), len(state.ActiveNodes), state.IsOperational); err != nil {
		return err
	}

	// Adopt first, then reconcile: an issuer the chain has and the platform
	// does not must appear before the status pass decides what is active, or
	// the console would report a group with a login method as having none.
	issuers := make([]string, 0, len(state.Issuers))
	for _, i := range state.Issuers {
		issuers = append(issuers, i.Issuer)
		if err := s.db.AdoptIssuer(ctx, app.ID, i.Issuer, chain.IssuerHash(i.Issuer),
			i.ClientIDs, guessProvider(i.Issuer)); err != nil {
			return err
		}
	}
	if err := s.db.SyncIssuerStatuses(ctx, app.ID, issuers); err != nil {
		return err
	}

	for _, k := range state.AuthKeys {
		keyHash, err := chain.AuthKeyHash(k)
		if err != nil {
			continue // a malformed key on-chain is the chain's problem, not a sync failure
		}
		if err := s.db.AdoptAuthKey(ctx, app.ID, k, keyHash); err != nil {
			return err
		}
	}
	return s.db.SyncAuthKeyStatuses(ctx, app.ID, state.AuthKeys)
}

// SyncAllGroups refreshes every live app's cached group state. Called on a
// timer by the server so the console is current without anyone opening it.
func (s *Server) SyncAllGroups(ctx context.Context) error {
	if !s.chain.Enabled() {
		return nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM apps WHERE group_address IS NOT NULL AND archived_at IS NULL`)
	if err != nil {
		return err
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	var firstErr error
	for _, id := range ids {
		app, err := s.db.App(ctx, id)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		// One unreachable group must not stop the rest from syncing.
		if err := s.syncGroup(ctx, app); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// developmentOperatorRefusal reports why a development app may not attach a
// given group, or "" when the group is acceptable.
//
// The operator set is read from the chain and classified against the directory.
// An operator the directory has never heard of counts as external: the platform
// cannot vouch for a node it has no record of, and treating an unknown as one
// of ours would be the wrong way to be wrong.
func (s *Server) developmentOperatorRefusal(r *http.Request, groupAddress string) (string, error) {
	state, err := s.chain.GroupState(r.Context(), groupAddress)
	if err != nil {
		return "", err
	}
	firstParty, err := s.db.FirstPartyOperatorAddresses(r.Context(), policy.FirstPartyCategory)
	if err != nil {
		return "", err
	}
	ours := make(map[string]bool, len(firstParty))
	for _, a := range firstParty {
		ours[strings.ToLower(a)] = true
	}

	external := []string{}
	for _, node := range state.ActiveNodes {
		if !ours[strings.ToLower(node)] {
			external = append(external, node)
		}
	}
	return policy.DeployableOperatorError(external), nil
}

// handleDeployGroup creates a signing group for an app.
//
// The developer's smart wallet is msg.sender, so `createGroup` makes them the
// manager in the same transaction that creates the group — there is no window
// in which the platform holds it and no separate handover to fail. That holds
// for every environment, so group management afterwards is the same operation
// everywhere.
//
// If the wallet has not been deployed yet, the console puts the factory's
// initCode on this operation: the CREATE2 deployment and the group creation
// land together, so a developer's first action prompts them for nothing.
//
// What the environment decides is who pays. A development group runs on our
// operators and our paymaster, which is why the operator set is restricted
// here — the platform will not fund a group of somebody else's nodes. A
// production group is the developer's own choice of operators and their own
// money, so the guard refuses an operation that arrives carrying paymaster
// data: sponsorship has to be something this route grants, not something a
// request can help itself to.
func (s *Server) handleDeployGroup(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}

	var req struct {
		Nodes           []string       `json:"nodes"`
		Threshold       int            `json:"threshold"`
		RemovalDelaySec int64          `json:"removal_delay_seconds"`
		UserOp          *userop.Packed `json:"user_op"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	app, err := s.db.App(r.Context(), access.AppID)
	if err != nil {
		writeStoreError(w, r, err, "could not load the app")
		return
	}
	if app.GroupAddress != nil {
		respond.Error(w, http.StatusConflict, "this app already has a signing group")
		return
	}
	// Sponsorship is the only thing the environment decides here. A production
	// group is still created by the developer's smart wallet and still passes
	// through this route — it just pays for itself, which is checked below
	// rather than assumed.
	sponsored := app.Environment == "development"

	nodes := req.Nodes
	if sponsored {
		// Only our own operators. The platform is paying, so it will not enrol
		// somebody else's nodes into a sponsored group on a developer's say-so.
		firstParty, err := s.db.FirstPartyOperatorAddresses(r.Context(), policy.FirstPartyCategory)
		if err != nil {
			respond.Fail(w, r, http.StatusInternalServerError, "could not read the operator directory", err)
			return
		}
		ours := make(map[string]bool, len(firstParty))
		for _, a := range firstParty {
			ours[strings.ToLower(a)] = true
		}
		if len(nodes) == 0 {
			nodes = firstParty // default to the whole first-party set
		}
		external := []string{}
		for _, n := range nodes {
			if !ours[strings.ToLower(n)] {
				external = append(external, n)
			}
		}
		if msg := policy.DeployableOperatorError(external); msg != "" {
			respond.Error(w, http.StatusConflict, msg)
			return
		}
		if len(nodes) == 0 {
			respond.Error(w, http.StatusConflict,
				"no O'Leary Labs operators are listed on this deployment, so there is nothing to deploy onto")
			return
		}
	} else if len(nodes) == 0 {
		respond.Error(w, http.StatusBadRequest,
			"choose the operators this production group will run on")
		return
	}

	threshold := req.Threshold
	if threshold <= 0 || threshold > len(nodes) {
		// A quorum of a first-party set: enough that one node being down does
		// not stop a developer, without pretending to a guarantee a group we
		// run alone cannot offer.
		threshold = (len(nodes) / 2) + 1
	}

	sender, _, err := s.smartWallet(r, id)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	receipt, ok := s.submitUserOp(w, r, req.UserOp, userop.Intent{
		Action:          "creating a signing group",
		Dest:            s.cfg.FactoryAddress,
		Selectors:       map[string]string{"createGroup": selCreateGroup},
		RefusePaymaster: !sponsored,
	}, sender)
	if !ok {
		return
	}

	// The address comes from the factory's own event rather than from anything
	// the console claimed, so what gets recorded is what was created.
	groupAddr, err := s.chain.GroupCreatedIn(r.Context(), receipt.TransactionHash)
	if err != nil {
		respond.Fail(w, r, http.StatusBadGateway,
			"the transaction went through but the new group could not be identified", err)
		return
	}
	if _, err := s.db.AttachGroup(r.Context(), access.AppID, groupAddr, "", threshold, len(nodes)); err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "the group was created but could not be linked", err)
		return
	}
	if app, err := s.db.App(r.Context(), access.AppID); err == nil {
		// Adopts the issuers and auth keys createGroup wrote, so the console
		// shows the group's real configuration rather than an empty one.
		_ = s.syncGroup(r.Context(), app)
	}

	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "app.group_deployed", Target: groupAddr,
		Metadata: map[string]any{
			"sponsored":        sponsored,
			"manager":          sender,
			"threshold":        threshold,
			"operators":        len(nodes),
			"transaction_hash": receipt.TransactionHash,
		},
		IP: r.RemoteAddr,
	})

	updated, _ := s.db.App(r.Context(), access.AppID)
	s.hooks.Emit(access.AppID, "app.deployed", updated)
	respond.JSON(w, http.StatusOK, map[string]any{
		"app":              updated,
		"group_address":    groupAddr,
		"manager":          sender,
		"transaction_hash": receipt.TransactionHash,
	})
}
