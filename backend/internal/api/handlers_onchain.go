package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/oleary-labs/signet-platform/backend/internal/auth"
	"github.com/oleary-labs/signet-platform/backend/internal/respond"
	"github.com/oleary-labs/signet-platform/backend/internal/store"
	"github.com/oleary-labs/signet-platform/backend/internal/structs"
	"github.com/oleary-labs/signet-platform/backend/internal/userop"
)

// Method selectors, computed from the contract signatures. They are listed here
// rather than derived at runtime so the set of calls the platform will sponsor
// is readable in one place — that list *is* the sponsorship policy.
const (
	selCreateGroup         = "73bc238e" // createGroup(address[],uint256,uint256,(string,string[])[],bytes[])
	selAddIssuer           = "f801f08c" // addIssuer(string,string[])
	selRemoveIssuer        = "1cc554f2" // removeIssuer(bytes32)
	selAddAuthKey          = "6d6a241f" // addAuthKey(bytes)
	selRemoveAuthKey       = "3e05d163" // removeAuthKey(bytes32)
	selInviteNode          = "bd758f63" // inviteNode(address)
	selQueueRemoval        = "6c036c01" // queueRemoval(address)
	selCancelRemoval       = "a56dc847" // cancelRemoval(address)
	selExecuteRemoval      = "e8a0894b" // executeRemoval(address)
	selTransferManager     = "ba0e930a" // transferManager(address)
	selRequestReshare      = "71cfccba" // requestReshare()
	selQueueAuthResolver   = "08507ebe" // queueAuthResolver(uint64,address,bool)
	selCancelAuthResolver  = "c25af104" // cancelAuthResolver()
	selExecuteAuthResolver = "d244ca42" // executeAuthResolver()
)

// groupManagementSelectors is everything a manager may do to their own group.
// The platform sponsors these because they are the actions the console exists
// to perform; anything else on the same contract is refused.
var groupManagementSelectors = map[string]string{
	"inviteNode":          selInviteNode,
	"queueRemoval":        selQueueRemoval,
	"cancelRemoval":       selCancelRemoval,
	"executeRemoval":      selExecuteRemoval,
	"transferManager":     selTransferManager,
	"requestReshare":      selRequestReshare,
	"addIssuer":           selAddIssuer,
	"removeIssuer":        selRemoveIssuer,
	"addAuthKey":          selAddAuthKey,
	"removeAuthKey":       selRemoveAuthKey,
	"queueAuthResolver":   selQueueAuthResolver,
	"cancelAuthResolver":  selCancelAuthResolver,
	"executeAuthResolver": selExecuteAuthResolver,
}

// smartWallet returns the caller's smart wallet, or an error explaining why
// they do not have one yet.
//
// The wallet is derived from the developer's Signet key, so not having one
// means the key was never provisioned — which is a sign-in problem, not
// something to paper over by falling back to another account.
func (s *Server) smartWallet(r *http.Request, id *auth.Identity) (string, *structs.User, error) {
	user, err := s.db.User(r.Context(), id.UserID)
	if err != nil {
		return "", nil, err
	}
	if user.SmartAccountAddress == nil || *user.SmartAccountAddress == "" {
		return "", user, fmt.Errorf(
			"your account has no smart wallet yet — sign out and back in so the console can provision your Signet key")
	}
	return *user.SmartAccountAddress, user, nil
}

// submitUserOp validates a signed operation against a route's intent and
// forwards it to the bundler.
//
// The two steps are inseparable: the platform is paying, so it decodes what it
// is being asked to pay for and refuses anything the route did not authorize.
// See internal/userop for the shape of that guard.
func (s *Server) submitUserOp(
	w http.ResponseWriter, r *http.Request,
	op *userop.Packed, intent userop.Intent, sender string,
) (*userop.Receipt, bool) {
	receipt, _, ok := s.submitUserOpDecoded(w, r, op, intent, sender, nil)
	return receipt, ok
}

// submitUserOpDecoded is submitUserOp with a route-supplied check on the
// decoded call, for routes that also care *which* record is being acted on and
// not only which method is being called.
func (s *Server) submitUserOpDecoded(
	w http.ResponseWriter, r *http.Request,
	op *userop.Packed, intent userop.Intent, sender string,
	check func(*userop.Decoded) error,
) (*userop.Receipt, *userop.Decoded, bool) {
	if s.bundler == nil {
		respond.Error(w, http.StatusServiceUnavailable,
			"this deployment has no bundler configured, so on-chain actions cannot be submitted")
		return nil, nil, false
	}
	decoded, err := userop.Validate(op, intent, sender)
	if err != nil {
		// A refusal here means the console sent something the route does not
		// authorize. That is worth surfacing verbatim: it is either a bug in
		// the console or an attempt to spend the paymaster on something else.
		respond.Error(w, http.StatusForbidden, err.Error())
		return nil, nil, false
	}
	if check != nil {
		if err := check(decoded); err != nil {
			respond.Error(w, http.StatusForbidden, err.Error())
			return nil, nil, false
		}
	}

	receipt, err := s.bundler.Send(r.Context(), op)
	if err != nil {
		respond.Fail(w, r, http.StatusBadGateway,
			fmt.Sprintf("the %s transaction could not be completed", intent.Action), err)
		return nil, nil, false
	}
	return receipt, decoded, true
}

// handleGroupExecute submits a management call against the caller's own group.
//
// One route covers membership, reshare, and resolver changes because they are
// one action in the product — "change my group" — and the guard is what keeps
// that from being a general-purpose sponsored relay: the destination must be
// this app's group, and the selector must be one a manager legitimately calls.
func (s *Server) handleGroupExecute(w http.ResponseWriter, r *http.Request) {
	id, access, ok := s.requireApp(w, r, auth.RoleDeveloper)
	if !ok {
		return
	}
	var req struct {
		UserOp *userop.Packed `json:"user_op"`
		Action string         `json:"action"`
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
	if app.GroupAddress == nil {
		respond.Error(w, http.StatusBadRequest, "this app has no signing group yet")
		return
	}
	sender, _, err := s.smartWallet(r, id)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	receipt, ok := s.submitUserOp(w, r, req.UserOp, userop.Intent{
		Action:    "group management",
		Dest:      *app.GroupAddress,
		Selectors: groupManagementSelectors,
		// Sponsorship follows the environment, the same way it does for group
		// creation. A production group is the developer's to run and to pay
		// for; without this, promoting an app to production would be a way to
		// keep free gas while leaving our operators behind.
		RefusePaymaster: app.Environment != "development",
	}, sender)
	if !ok {
		return
	}

	// Re-read the group so the console shows the change rather than the state
	// it had a moment ago.
	if err := s.syncGroup(r.Context(), app); err != nil {
		respond.JSON(w, http.StatusOK, map[string]any{
			"transaction_hash": receipt.TransactionHash,
			"sync_error":       err.Error(),
		})
		return
	}

	action := req.Action
	if action == "" {
		action = "group.updated"
	}
	s.db.Audit(r.Context(), store.AuditRecord{
		OrgID: &access.OrgID, AppID: &access.AppID, ActorID: &id.UserID, ActorLabel: id.Subject,
		Action: "group." + strings.ToLower(action), Target: *app.GroupAddress,
		Metadata: map[string]any{"transaction_hash": receipt.TransactionHash, "sender": sender},
		IP:       r.RemoteAddr,
	})

	nodes, _ := s.db.GroupNodes(r.Context(), access.AppID)
	updated, _ := s.db.App(r.Context(), access.AppID)
	respond.JSON(w, http.StatusOK, map[string]any{
		"transaction_hash": receipt.TransactionHash,
		"app":              updated,
		"nodes":            nodes,
	})
}
