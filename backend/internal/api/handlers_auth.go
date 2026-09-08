package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/oleary-labs/signet-platform/backend/internal/auth"
	"github.com/oleary-labs/signet-platform/backend/internal/policy"
	"github.com/oleary-labs/signet-platform/backend/internal/respond"
	"github.com/oleary-labs/signet-platform/backend/internal/store"
	"github.com/oleary-labs/signet-platform/backend/internal/structs"
)

// challengeTTL is how long a login nonce stays usable. Long enough for a
// threshold signing round-trip across geographically spread nodes, short
// enough that a captured challenge is worthless by the time it is replayed.
const challengeTTL = 5 * time.Minute

// handleAuthChallenge issues a single-use login nonce.
//
// The platform dogfoods Signet: the `signet` method asks the developer's own
// bootstrap signing group to threshold-sign this nonce, and the server
// verifies that FROST signature. The `siwe` method is the EOA route, for node
// operators and teams who have not onboarded through Signet.
func (s *Server) handleAuthChallenge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Method  string `json:"method"`
		Address string `json:"address"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Method == "" {
		req.Method = "signet"
	}
	if req.Method != "signet" && req.Method != "siwe" {
		respond.Error(w, http.StatusBadRequest, `method must be "signet" or "siwe"`)
		return
	}
	if req.Method == "siwe" && s.cfg.SIWEDomain == "" {
		respond.Error(w, http.StatusBadRequest, "SIWE login is not enabled on this server")
		return
	}

	nonce, err := store.RandomToken(16)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not start sign-in", err)
		return
	}

	address := ""
	if req.Address != "" {
		normalized, err := auth.NormalizeAddress(req.Address)
		if err != nil {
			respond.Error(w, http.StatusBadRequest, "invalid address")
			return
		}
		address = normalized
	}

	statement := "Sign in to the Signet platform console."
	if err := s.db.CreateChallenge(r.Context(), nonce, req.Method, address, statement, challengeTTL); err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not start sign-in", err)
		return
	}

	out := map[string]any{
		"nonce":      nonce,
		"method":     req.Method,
		"statement":  statement,
		"expires_at": time.Now().Add(challengeTTL).UTC(),
	}
	if req.Method == "signet" {
		// The developer signs the digest of this exact string with their
		// SignetAccount's group key. Naming the domain and nonce inside the
		// signed material is what stops a signature made for another site — or
		// another login — being replayed here.
		out["message"] = signetLoginMessage(s.cfg.PublicWebURL, nonce)
		out["digest"] = "0x" + hexDigest(signetLoginMessage(s.cfg.PublicWebURL, nonce))
	} else {
		out["domain"] = s.cfg.SIWEDomain
		out["chain_id"] = s.cfg.ChainID
		out["uri"] = s.cfg.PublicWebURL
		out["issued_at"] = time.Now().UTC().Format(time.RFC3339)
	}
	respond.JSON(w, http.StatusOK, out)
}

// signetLoginMessage is the exact string a Signet-route login signs.
func signetLoginMessage(origin, nonce string) string {
	return fmt.Sprintf("Signet Platform sign-in\norigin: %s\nnonce: %s", strings.TrimRight(origin, "/"), nonce)
}

// hexDigest is the keccak256 of a login message, hex-encoded. It is returned
// alongside the message purely so a client can check it computed the same
// digest before asking the nodes to sign it.
func hexDigest(message string) string {
	sum := auth.Keccak256([]byte(message))
	const digits = "0123456789abcdef"
	out := make([]byte, len(sum)*2)
	for i, b := range sum {
		out[i*2] = digits[b>>4]
		out[i*2+1] = digits[b&0x0f]
	}
	return string(out)
}

// handleAuthVerify checks a signed challenge and opens a session.
func (s *Server) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Method         string `json:"method"`
		Nonce          string `json:"nonce"`
		Signature      string `json:"signature"`
		GroupPublicKey string `json:"group_public_key"`
		Account        string `json:"account"`
		Message        string `json:"message"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Method == "" {
		req.Method = "signet"
	}

	// Consuming the nonce first means a signature is only ever checked once
	// per challenge, whether or not it turns out to be valid.
	if _, err := s.db.ConsumeChallenge(r.Context(), req.Nonce, req.Method); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respond.Error(w, http.StatusUnauthorized, "that sign-in challenge is expired or already used")
			return
		}
		respond.Fail(w, r, http.StatusInternalServerError, "could not complete sign-in", err)
		return
	}

	var subject, accountAddress, groupPublicKey string

	switch req.Method {
	case "signet":
		sig, err := auth.DecodeHex(req.Signature)
		if err != nil {
			respond.Error(w, http.StatusBadRequest, "signature is not hex")
			return
		}
		groupKey, err := auth.DecodeHex(req.GroupPublicKey)
		if err != nil {
			respond.Error(w, http.StatusBadRequest, "group public key is not hex")
			return
		}
		message := signetLoginMessage(s.cfg.PublicWebURL, req.Nonce)
		if err := auth.VerifyFROST(auth.Keccak256([]byte(message)), sig, groupKey); err != nil {
			respond.Error(w, http.StatusUnauthorized, "that signature does not verify against the group key")
			return
		}
		// The account address the client claims is bound to the group key it
		// just proved control of, so it is recorded for display but is not
		// what identifies the user — the group key is.
		if req.Account != "" {
			normalized, err := auth.NormalizeAddress(req.Account)
			if err != nil {
				respond.Error(w, http.StatusBadRequest, "invalid account address")
				return
			}
			accountAddress = normalized
		}
		groupPublicKey = strings.ToLower(strings.TrimPrefix(req.GroupPublicKey, "0x"))
		subject = "signet:" + groupPublicKey

	case "siwe":
		sig, err := auth.DecodeHex(req.Signature)
		if err != nil {
			respond.Error(w, http.StatusBadRequest, "signature is not hex")
			return
		}
		msg, err := auth.VerifySIWE(req.Message, sig, auth.SIWEExpectation{
			Domain:  s.cfg.SIWEDomain,
			Nonce:   req.Nonce,
			ChainID: s.cfg.ChainID,
		})
		if err != nil {
			respond.Error(w, http.StatusUnauthorized, "sign-in message rejected: "+err.Error())
			return
		}
		subject = "eth:" + msg.Address
		accountAddress = msg.Address

	default:
		respond.Error(w, http.StatusBadRequest, `method must be "signet" or "siwe"`)
		return
	}

	// Staff is decided from the subject the signature just established, so a
	// newly listed operator is staff on their next sign-in rather than on the
	// next deploy. The upsert only ever ORs this in; revocation is handled by
	// the reconciliation at boot.
	user, err := s.db.UpsertUser(r.Context(), subject, methodKind(req.Method), accountAddress, groupPublicKey,
		s.cfg.IsStaffSubject(subject))
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not open your account", err)
		return
	}

	token, expiresAt, err := s.sessions.Issue(user.ID, user.Subject, req.Method)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not issue a session", err)
		return
	}
	s.setSessionCookie(w, token, expiresAt)

	s.db.Audit(r.Context(), store.AuditRecord{
		ActorID:    &user.ID,
		ActorLabel: user.Subject,
		Action:     "auth.signed_in",
		Target:     user.Subject,
		Metadata:   map[string]any{"method": req.Method},
		IP:         r.RemoteAddr,
	})

	session, err := s.buildSession(r, user)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not load your account", err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"session":    session,
		"token":      token,
		"expires_at": expiresAt,
	})
}

func methodKind(method string) string {
	if method == "siwe" {
		return "siwe"
	}
	return "signet"
}

// handleLogout clears the session cookie. Sessions are stateless, so this
// cannot revoke a bearer token already in someone's hands — the short TTL is
// what bounds that, and the console says so on the sign-out screen.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.setSessionCookie(w, "", time.Unix(0, 0))
	respond.JSON(w, http.StatusOK, map[string]string{"status": "signed out"})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	cookie := &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		Domain:   s.cfg.CookieDomain,
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	if token == "" {
		cookie.MaxAge = -1
	}
	http.SetCookie(w, cookie)
}

// handleGetMe returns the signed-in developer, their organizations, and the
// protocol wiring the console needs to build UserOperations itself.
func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	id, ok := identity(w, r)
	if !ok {
		return
	}
	user, err := s.db.User(r.Context(), id.UserID)
	if err != nil {
		writeStoreError(w, r, err, "could not load your account")
		return
	}
	session, err := s.buildSession(r, user)
	if err != nil {
		respond.Fail(w, r, http.StatusInternalServerError, "could not load your account", err)
		return
	}
	respond.JSON(w, http.StatusOK, session)
}

func (s *Server) buildSession(r *http.Request, user *structs.User) (*structs.Session, error) {
	orgs, err := s.db.OrgsForUser(r.Context(), user.ID)
	if err != nil {
		return nil, err
	}
	return &structs.Session{
		User:          *user,
		Organizations: orgs,
		Network:       s.networkConfig(),
	}, nil
}

func (s *Server) networkConfig() structs.NetworkConfig {
	return structs.NetworkConfig{
		ChainID:               s.cfg.ChainID,
		// The public endpoint, not the one this server reads through — see
		// Config.PublicRPCURL. This response is unauthenticated.
		RPCURL:                s.cfg.PublicRPCURL,
		FactoryAddress:        s.cfg.FactoryAddress,
		AccountFactoryAddress: s.cfg.AccountFactoryAddress,
		EntryPointAddress:     s.cfg.EntryPointAddress,
		BundlerURL:            s.cfg.BundlerURL,
		BootstrapGroup:        s.cfg.BootstrapGroup,
		BootstrapNodes:        s.cfg.BootstrapNodes,
		SIWEEnabled:           s.cfg.SIWEDomain != "",

		// Sponsorship needs somewhere to ask. Reporting it as on without a
		// paymaster configured would have the console attach sponsorship data
		// that nothing can sign.
		PaymasterURL:         s.cfg.PaymasterURL,
		PaymasterAddress:     s.cfg.PaymasterAddress,
		SponsorGroupCreation: s.cfg.SponsorGroupCreation && s.cfg.PaymasterURL != "",

		FirstPartyCategory: policy.FirstPartyCategory,
		DevelopmentPolicy: "Development groups run on O'Leary Labs operators only and their " +
			"deployment gas is sponsored. That makes a development group single-operator, " +
			"which is why production requires operators outside O'Leary Labs.",
	}
}

// handlePublicConfig serves the network wiring to signed-out visitors, so the
// login screen can talk to the bootstrap group before a session exists.
func (s *Server) handlePublicConfig(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusOK, s.networkConfig())
}

// handleUpdateMe edits the developer's profile fields.
func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	id, ok := identity(w, r)
	if !ok {
		return
	}
	var req struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		AvatarURL   string `json:"avatar_url"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, err := s.db.UpdateProfile(r.Context(), id.UserID, req.DisplayName, req.Email, req.AvatarURL)
	if err != nil {
		writeStoreError(w, r, err, "could not save your profile")
		return
	}
	respond.JSON(w, http.StatusOK, user)
}

// handleNodeCertificate issues an auth-key certificate for the signed-in
// developer, so their browser can open a session with the platform's signing
// group and hold a Signet key.
//
// This is what makes the login method irrelevant to what a developer ends up
// with. A wallet signature is an auth guard: it proves who they are once, and
// the platform then vouches for them to its own group. From that point on the
// Signet key signs, and MetaMask is never opened again.
//
// The certificate is bound to the exact session public key the client just
// generated and expires in an hour, so it is not a credential worth capturing.
func (s *Server) handleNodeCertificate(w http.ResponseWriter, r *http.Request) {
	id, ok := identity(w, r)
	if !ok {
		return
	}
	if s.certs == nil {
		respond.Error(w, http.StatusServiceUnavailable,
			"this deployment has no platform auth key configured, so it cannot issue node certificates")
		return
	}

	var req struct {
		SessionPub string `json:"session_pub"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// The identity is the platform subject, so every developer gets their own
	// key namespace inside the group and nobody can request another's.
	cert, err := s.certs.Issue(id.Subject, req.SessionPub)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	respond.JSON(w, http.StatusOK, map[string]any{
		"certificate": cert,
		"group_id":    s.certs.GroupID(),
		"node_urls":   s.cfg.BootstrapNodes,
		"identity":    id.Subject,
	})
}

// handleRecordSignetKey stores the key the console just generated, and the
// smart wallet it controls.
//
// The console derives both — the key from a distributed generation round it
// drove, the wallet address CREATE2 from the resulting group public key — and
// reports them here. The platform records rather than derives: it has no
// session with the nodes and could not have produced either.
func (s *Server) handleRecordSignetKey(w http.ResponseWriter, r *http.Request) {
	id, ok := identity(w, r)
	if !ok {
		return
	}
	var req struct {
		GroupPublicKey string `json:"group_public_key"`
		KeyID          string `json:"key_id"`
		KeyAddress     string `json:"key_address"`
		SmartAccount   string `json:"smart_account"`
	}
	if err := respond.Decode(r, &req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.GroupPublicKey) == "" {
		respond.Error(w, http.StatusBadRequest, "group_public_key is required")
		return
	}

	// The wallet address decides which operations the platform will pay for, so
	// it is derived here rather than believed. The factory computes the same
	// CREATE2 address the EntryPoint will deploy to, which also means a
	// developer cannot claim someone else's wallet — the key determines the
	// address, and they do not hold that key.
	smart := ""
	if s.chain.Enabled() && s.cfg.AccountFactoryAddress != "" {
		derived, err := s.chain.SmartAccountAddress(r.Context(),
			s.cfg.AccountFactoryAddress, s.cfg.EntryPointAddress, req.GroupPublicKey)
		if err != nil {
			respond.Fail(w, r, http.StatusBadGateway,
				"could not derive your smart wallet address from the account factory", err)
			return
		}
		smart = strings.ToLower(derived)

		if req.SmartAccount != "" && !strings.EqualFold(strings.TrimSpace(req.SmartAccount), derived) {
			// Not fatal, but worth recording: the console derived a different
			// address from the same key, which means one of the two is using
			// the wrong factory or entry point.
			slog.Warn("smart account mismatch between console and factory",
				"reported", req.SmartAccount, "derived", derived, "user", id.UserID)
		}
	} else if req.SmartAccount != "" {
		// No chain configured — the console's value is all there is. It is
		// unverified, and the operation guard is correspondingly weaker.
		normalized, err := auth.NormalizeAddress(req.SmartAccount)
		if err != nil {
			respond.Error(w, http.StatusBadRequest, "invalid smart account address")
			return
		}
		smart = normalized
	}
	keyAddr := ""
	if req.KeyAddress != "" {
		normalized, err := auth.NormalizeAddress(req.KeyAddress)
		if err != nil {
			respond.Error(w, http.StatusBadRequest, "invalid key address")
			return
		}
		keyAddr = normalized
	}

	user, err := s.db.RecordSignetKey(r.Context(), id.UserID,
		strings.ToLower(strings.TrimPrefix(req.GroupPublicKey, "0x")), req.KeyID, keyAddr, smart)
	if err != nil {
		writeStoreError(w, r, err, "could not record your Signet key")
		return
	}
	respond.JSON(w, http.StatusOK, user)
}
