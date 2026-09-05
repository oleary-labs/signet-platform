// Package structs holds the JSON shapes the API returns. Keeping them in one
// place makes the console's TypeScript types (web/lib/types.ts) a direct,
// checkable mirror of the server's contract.
package structs

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ─────────────────────────── Identity ───────────────────────────

// User is the signed-in developer.
type User struct {
	ID             uuid.UUID `json:"id"`
	Subject        string    `json:"subject"`
	SubjectKind    string    `json:"subject_kind"`
	AccountAddress *string   `json:"account_address"`
	GroupPublicKey *string   `json:"group_public_key"`
	Email          *string   `json:"email"`
	DisplayName    string    `json:"display_name"`
	AvatarURL      *string   `json:"avatar_url"`
	IsStaff        bool      `json:"is_staff"`
	// The smart wallet the Signet key controls, and the key itself. Every
	// on-chain action the console takes is a UserOperation from this wallet.
	SmartAccountAddress *string    `json:"smart_account_address"`
	SignetKeyID         *string    `json:"signet_key_id"`
	SignetKeyAddress    *string    `json:"signet_key_address"`
	LastSeenAt          *time.Time `json:"last_seen_at"`
	CreatedAt           time.Time  `json:"created_at"`
}

// Session is what /v1/me returns: the user plus everything the console needs
// to render its shell without a second round-trip.
type Session struct {
	User          User           `json:"user"`
	Organizations []Organization `json:"organizations"`
	Network       NetworkConfig  `json:"network"`
}

// NetworkConfig is the protocol wiring the console needs to build and submit
// UserOperations itself. The platform hands these out but never uses them to
// sign anything.
type NetworkConfig struct {
	ChainID               int64    `json:"chain_id"`
	RPCURL                string   `json:"rpc_url"`
	FactoryAddress        string   `json:"factory_address"`
	AccountFactoryAddress string   `json:"account_factory_address"`
	EntryPointAddress     string   `json:"entrypoint_address"`
	BundlerURL            string   `json:"bundler_url"`
	BootstrapGroup        string   `json:"bootstrap_group"`
	BootstrapNodes        []string `json:"bootstrap_nodes"`
	SIWEEnabled           bool     `json:"siwe_enabled"`

	// Gas sponsorship for group creation. The console reads these to decide
	// whether to attach paymaster data to the deploy UserOperation; sponsorship
	// is only possible on that path, since a transaction sent straight from a
	// developer's own wallet pays its own gas by construction.
	PaymasterURL         string `json:"paymaster_url"`
	PaymasterAddress     string `json:"paymaster_address"`
	SponsorGroupCreation bool   `json:"sponsor_group_creation"`

	// The operator category the platform treats as first-party, and a plain
	// statement of what that buys and costs in a development environment.
	FirstPartyCategory string `json:"first_party_category"`
	DevelopmentPolicy  string `json:"development_policy"`
}

// ─────────────────────────── Organizations ───────────────────────────

// Organization is a billing and membership boundary. Apps belong to exactly
// one; people are members of the org, never of an individual app.
type Organization struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	LogoURL      *string   `json:"logo_url"`
	WebsiteURL   *string   `json:"website_url"`
	BillingEmail *string   `json:"billing_email"`
	Plan         string    `json:"plan"`
	// Role is the requesting user's role; empty when listing as staff.
	Role      string    `json:"role,omitempty"`
	AppCount  int       `json:"app_count"`
	CreatedAt time.Time `json:"created_at"`
}

// OrgMember is one person's membership.
type OrgMember struct {
	UserID      uuid.UUID `json:"user_id"`
	Subject     string    `json:"subject"`
	Email       *string   `json:"email"`
	DisplayName string    `json:"display_name"`
	AvatarURL   *string   `json:"avatar_url"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

// OrgInvite is a pending invitation. The raw token is returned exactly once,
// at creation, and only its hash is stored.
type OrgInvite struct {
	ID         uuid.UUID  `json:"id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	Token      string     `json:"token,omitempty"`
	AcceptURL  string     `json:"accept_url,omitempty"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
}

// ─────────────────────────── Apps ───────────────────────────

// App is a developer's project, backed by one on-chain signing group.
type App struct {
	ID             uuid.UUID  `json:"id"`
	OrgID          uuid.UUID  `json:"org_id"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	Description    string     `json:"description"`
	Environment    string     `json:"environment"`
	Status         string     `json:"status"`
	ChainID        int64      `json:"chain_id"`
	GroupAddress   *string    `json:"group_address"`
	GroupPublicKey *string    `json:"group_public_key"`
	Threshold      *int       `json:"threshold"`
	NodeCount      *int       `json:"node_count"`
	IsOperational  *bool      `json:"is_operational"`
	LogoURL        *string    `json:"logo_url"`
	WebsiteURL     *string    `json:"website_url"`
	DeployedAt     *time.Time `json:"deployed_at"`
	SyncedAt       *time.Time `json:"synced_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// AppSettings is the configuration surface: one JSON document per console
// screen, so saving one screen can never clobber another.
type AppSettings struct {
	LoginMethods    json.RawMessage `json:"login_methods"`
	Branding        json.RawMessage `json:"branding"`
	EmbeddedWallets json.RawMessage `json:"embedded_wallets"`
	SmartAccounts   json.RawMessage `json:"smart_accounts"`
	SessionSigners  json.RawMessage `json:"session_signers"`
	Funding         json.RawMessage `json:"funding"`
	Compliance      json.RawMessage `json:"compliance"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// AppDomain is an origin permitted to use the app's client SDK.
type AppDomain struct {
	ID         uuid.UUID  `json:"id"`
	Origin     string     `json:"origin"`
	VerifiedAt *time.Time `json:"verified_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Credential is either a platform app secret or a mirrored on-chain
// authorization key. Private key material is never stored for either.
type Credential struct {
	ID            uuid.UUID  `json:"id"`
	Kind          string     `json:"kind"`
	Label         string     `json:"label"`
	LastFour      *string    `json:"last_four"`
	PublicKey     *string    `json:"public_key"`
	KeyHash       *string    `json:"key_hash"`
	OnchainStatus string     `json:"onchain_status"`
	LastUsedAt    *time.Time `json:"last_used_at"`
	CreatedAt     time.Time  `json:"created_at"`
	// Secret is populated only in the response that creates it.
	Secret string `json:"secret,omitempty"`
}

// Issuer is a trusted OAuth/OIDC issuer, with the console metadata the chain
// has no room to carry.
type Issuer struct {
	ID            uuid.UUID `json:"id"`
	Issuer        string    `json:"issuer"`
	IssuerHash    *string   `json:"issuer_hash"`
	ClientIDs     []string  `json:"client_ids"`
	Provider      string    `json:"provider"`
	Label         string    `json:"label"`
	Enabled       bool      `json:"enabled"`
	OnchainStatus string    `json:"onchain_status"`
	CreatedAt     time.Time `json:"created_at"`
}

// ─────────────────────────── Nodes ───────────────────────────

// NodeOperator is a marketplace listing: curated off-chain metadata joined to
// the on-chain registry entry and recent liveness.
type NodeOperator struct {
	Address         string      `json:"address"`
	Name            string      `json:"name"`
	Slug            string      `json:"slug"`
	Description     string      `json:"description"`
	WebsiteURL      *string     `json:"website_url"`
	LogoURL         *string     `json:"logo_url"`
	APIURL          *string     `json:"api_url"`
	Region          string      `json:"region"`
	Jurisdiction    string      `json:"jurisdiction"`
	Category        string      `json:"category"`
	Verified        bool        `json:"verified"`
	IsOpen          *bool       `json:"is_open"`
	RegisteredAt    *time.Time  `json:"registered_at"`
	OperatorAddress *string     `json:"operator_address"`
	GroupCount      int         `json:"group_count"`
	SyncedAt        *time.Time  `json:"synced_at"`
	Health          *NodeHealth `json:"health"`
}

// NodeHealth summarises recent probe results for a node.
type NodeHealth struct {
	Online       bool       `json:"online"`
	LatencyMS    *int       `json:"latency_ms"`
	PeerCount    *int       `json:"peer_count"`
	UptimePct24h *float64   `json:"uptime_pct_24h"`
	LastError    *string    `json:"last_error"`
	ObservedAt   *time.Time `json:"observed_at"`
}

// GroupNode is one node's standing inside an app's group.
type GroupNode struct {
	Address      string        `json:"address"`
	Status       string        `json:"status"`
	ExecuteAfter *time.Time    `json:"execute_after"`
	Initiator    *string       `json:"removal_initiator"`
	JoinedAt     *time.Time    `json:"joined_at"`
	Operator     *NodeOperator `json:"operator"`
}

// ─────────────────────────── Keys and users ───────────────────────────

// Key is one threshold key held by an app's group.
type Key struct {
	ID            uuid.UUID  `json:"id"`
	KeyID         string     `json:"key_id"`
	Curve         string     `json:"curve"`
	PublicKey     *string    `json:"public_key"`
	Address       *string    `json:"address"`
	ScopeHex      *string    `json:"scope_hex"`
	ScopeKind     string     `json:"scope_kind"`
	ScopeChainID  *int64     `json:"scope_chain_id"`
	ScopeContract *string    `json:"scope_contract"`
	ScopeTypeHash *string    `json:"scope_type_hash"`
	ParentKeyID   *string    `json:"parent_key_id"`
	Threshold     *int       `json:"threshold"`
	Parties       []string   `json:"parties"`
	Status        string     `json:"status"`
	Label         string     `json:"label"`
	SubjectHash   *string    `json:"subject_hash"`
	LastUsedAt    *time.Time `json:"last_used_at"`
	SyncedAt      time.Time  `json:"synced_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// AppUser is an end user of a developer's app, identified only by a hash of
// (issuer, subject). The platform never receives the raw subject.
type AppUser struct {
	ID          uuid.UUID `json:"id"`
	SubjectHash string    `json:"subject_hash"`
	Issuer      string    `json:"issuer"`
	Label       string    `json:"label"`
	KeyCount    int       `json:"key_count"`
	Status      string    `json:"status"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

// Delegation is a session signer: a long-lived grant letting an agent use one
// scoped sub-key without the user's OAuth session.
type Delegation struct {
	ID          uuid.UUID  `json:"id"`
	KeyID       string     `json:"key_id"`
	ParentKeyID string     `json:"parent_key_id"`
	Curve       string     `json:"curve"`
	Label       string     `json:"label"`
	SubjectHash *string    `json:"subject_hash"`
	IssuedAt    time.Time  `json:"issued_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
}

// Policy is a developer-defined rule. EnforcedBy is deliberately explicit: a
// key scope is checked by every node, a smart-account rule by the chain, and
// anything marked advisory is recorded here but enforced by the app itself.
type Policy struct {
	ID         uuid.UUID       `json:"id"`
	Name       string          `json:"name"`
	Kind       string          `json:"kind"`
	EnforcedBy string          `json:"enforced_by"`
	Config     json.RawMessage `json:"config"`
	Enabled    bool            `json:"enabled"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// ─────────────────────────── Webhooks ───────────────────────────

// Webhook is an HTTP endpoint the platform posts app events to.
type Webhook struct {
	ID          uuid.UUID `json:"id"`
	URL         string    `json:"url"`
	Events      []string  `json:"events"`
	Enabled     bool      `json:"enabled"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	// Secret is returned only when the webhook is created.
	Secret string `json:"secret,omitempty"`
}

// WebhookDelivery is one attempt to deliver an event.
type WebhookDelivery struct {
	ID         uuid.UUID       `json:"id"`
	Event      string          `json:"event"`
	Payload    json.RawMessage `json:"payload"`
	StatusCode *int            `json:"status_code"`
	Error      *string         `json:"error"`
	Attempt    int             `json:"attempt"`
	DurationMS *int            `json:"duration_ms"`
	CreatedAt  time.Time       `json:"created_at"`
}

// ─────────────────────────── Usage and billing ───────────────────────────

// UsagePoint is one day of an app's metered activity.
type UsagePoint struct {
	Day           string `json:"day"`
	ActiveWallets int    `json:"active_wallets"`
	AuthCount     int    `json:"auth_count"`
	KeygenCount   int    `json:"keygen_count"`
	SignCount     int    `json:"sign_count"`
	ErrorCount    int    `json:"error_count"`
	P50LatencyMS  *int   `json:"p50_latency_ms"`
	P95LatencyMS  *int   `json:"p95_latency_ms"`
}

// UsageSummary is the analytics screen's headline figures plus its series.
type UsageSummary struct {
	From                 string       `json:"from"`
	To                   string       `json:"to"`
	MonthlyActiveWallets int          `json:"monthly_active_wallets"`
	TotalSigns           int          `json:"total_signs"`
	TotalKeygens         int          `json:"total_keygens"`
	TotalAuths           int          `json:"total_auths"`
	ErrorRate            float64      `json:"error_rate"`
	Series               []UsagePoint `json:"series"`
}

// BillingAccount is an org's balance and rate. Payments are not switched on
// yet: no code path debits this, and Active reports that plainly.
type BillingAccount struct {
	Currency           string    `json:"currency"`
	BalanceMicros      int64     `json:"balance_micros"`
	RateMicrosPerMAW   int64     `json:"rate_micros_per_maw"`
	LowBalanceMicros   int64     `json:"low_balance_micros"`
	ContractAddress    *string   `json:"contract_address"`
	ChainID            *int64    `json:"chain_id"`
	Status             string    `json:"status"`
	PaymentsEnabled    bool      `json:"payments_enabled"`
	EstimatedMonthMAW  int       `json:"estimated_month_maw"`
	EstimatedMonthCost int64     `json:"estimated_month_cost_micros"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Invoice is a settled or draft billing period.
type Invoice struct {
	ID               uuid.UUID  `json:"id"`
	PeriodStart      string     `json:"period_start"`
	PeriodEnd        string     `json:"period_end"`
	ActiveWallets    int        `json:"active_wallets"`
	RateMicrosPerMAW int64      `json:"rate_micros_per_maw"`
	AmountMicros     int64      `json:"amount_micros"`
	Status           string     `json:"status"`
	IssuedAt         *time.Time `json:"issued_at"`
	PaidAt           *time.Time `json:"paid_at"`
}

// ─────────────────────────── Assets and audit ───────────────────────────

// Asset is one stored upload.
type Asset struct {
	ID          uuid.UUID  `json:"id"`
	AppID       *uuid.UUID `json:"app_id"`
	Kind        string     `json:"kind"`
	Scope       string     `json:"scope"`
	URL         string     `json:"url"`
	ContentType string     `json:"content_type"`
	ByteSize    int64      `json:"byte_size"`
	CreatedAt   time.Time  `json:"created_at"`
}

// AuditEntry is one recorded action.
type AuditEntry struct {
	ID         int64           `json:"id"`
	AppID      *uuid.UUID      `json:"app_id"`
	ActorLabel string          `json:"actor_label"`
	Action     string          `json:"action"`
	Target     string          `json:"target"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"created_at"`
}
