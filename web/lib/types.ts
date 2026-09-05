// Mirrors backend/internal/structs. Keep the two in step: the console reads
// these fields directly, so a rename on either side is a silent break.

export type Role = "owner" | "admin" | "developer" | "viewer";
export type Environment = "development" | "production";
export type AppStatus = "provisioning" | "live" | "suspended" | "archived";
export type Curve = "frost_secp256k1" | "frost_ed25519" | "ecdsa_secp256k1";
export type ScopeKind = "unscoped" | "evm_userop" | "solana_tx" | "eip712";
export type OnchainStatus = "unknown" | "pending" | "active" | "removed";

export interface User {
  id: string;
  subject: string;
  subject_kind: "signet" | "siwe";
  account_address: string | null;
  group_public_key: string | null;
  /** The smart wallet every on-chain action is sent from. CREATE2-derived from
   *  the Signet key below, so it has an address before it is deployed — its
   *  first operation deploys it. */
  smart_account_address: string | null;
  /** The Signet key that controls that wallet. Present however the developer
   *  signed in: a wallet sign-in is only an auth guard for obtaining it. */
  signet_key_id: string | null;
  signet_key_address: string | null;
  email: string | null;
  display_name: string;
  avatar_url: string | null;
  is_staff: boolean;
  last_seen_at: string | null;
  created_at: string;
}

export interface NetworkConfig {
  chain_id: number;
  rpc_url: string;
  factory_address: string;
  account_factory_address: string;
  entrypoint_address: string;
  bundler_url: string;
  bootstrap_group: string;
  bootstrap_nodes: string[];
  siwe_enabled: boolean;
  /** Gas sponsorship for group creation — only possible on the UserOperation
   *  path; a transaction from your own wallet pays its own gas. */
  paymaster_url: string;
  paymaster_address: string;
  sponsor_group_creation: boolean;
  /** The operator category the platform treats as first-party. */
  first_party_category: string;
  development_policy: string;
}

export interface EnvironmentRequirement {
  id: string;
  label: string;
  detail: string;
  met: boolean;
  blocking: boolean;
}

export interface EnvironmentCheck {
  from: string;
  to: string;
  allowed: boolean;
  requirements: EnvironmentRequirement[] | null;
  note: string;
}

export interface Organization {
  id: string;
  name: string;
  slug: string;
  logo_url: string | null;
  website_url: string | null;
  billing_email: string | null;
  plan: "developer" | "team" | "enterprise";
  role?: Role;
  app_count: number;
  created_at: string;
}

export interface Session {
  user: User;
  organizations: Organization[];
  network: NetworkConfig;
}

export interface OrgMember {
  user_id: string;
  subject: string;
  email: string | null;
  display_name: string;
  avatar_url: string | null;
  role: Role;
  created_at: string;
}

export interface OrgInvite {
  id: string;
  email: string;
  role: Role;
  expires_at: string;
  created_at: string;
  token?: string;
  accept_url?: string;
  accepted_at?: string | null;
}

export interface App {
  id: string;
  org_id: string;
  name: string;
  slug: string;
  description: string;
  environment: Environment;
  status: AppStatus;
  chain_id: number;
  group_address: string | null;
  group_public_key: string | null;
  threshold: number | null;
  node_count: number | null;
  is_operational: boolean | null;
  logo_url: string | null;
  website_url: string | null;
  deployed_at: string | null;
  synced_at: string | null;
  created_at: string;
  updated_at: string;
}

// Each settings section is a free-form document owned by one console screen.
export interface AppSettings {
  login_methods: LoginMethodSettings;
  branding: BrandingSettings;
  embedded_wallets: EmbeddedWalletSettings;
  smart_accounts: SmartAccountSettings;
  session_signers: SessionSignerSettings;
  funding: FundingSettings;
  compliance: ComplianceSettings;
  updated_at: string;
}

export interface LoginMethodSettings {
  /** Issuers are authoritative on-chain; these toggles drive the login modal. */
  google?: boolean;
  apple?: boolean;
  email?: boolean;
  sms?: boolean;
  passkey?: boolean;
  wallet?: boolean;
  farcaster?: boolean;
  telegram?: boolean;
  guest?: boolean;
  custom_jwt?: boolean;
  /** Auth-key certificates: server-to-server sessions for your own backend. */
  auth_key?: boolean;
  /** SIWE against an on-chain resolver bound to the group. */
  onchain_resolver?: boolean;
  primary?: string[];
}

export interface BrandingSettings {
  theme?: "light" | "dark" | "system";
  accent_color?: string;
  logo_url?: string;
  app_name?: string;
  header_text?: string;
  login_message?: string;
  terms_url?: string;
  privacy_url?: string;
  show_wallet_ui?: boolean;
  corner_radius?: "sharp" | "soft" | "round";
}

export interface EmbeddedWalletSettings {
  creation?: "on_login" | "on_demand" | "off";
  curves?: Curve[];
  default_curve?: Curve;
  require_confirmation_on_sign?: boolean;
  require_confirmation_on_transaction?: boolean;
  prover?: "client" | "server";
  session_ttl_seconds?: number;
  default_chain_id?: number;
  chain_ids?: number[];
}

export interface SmartAccountSettings {
  enabled?: boolean;
  implementation?: "signet_kernel" | "signet_minimal" | "external";
  entrypoint?: string;
  bundler_url?: string;
  paymaster_url?: string;
  paymaster_enabled?: boolean;
  sponsor_first_deployment?: boolean;
}

export interface SessionSignerSettings {
  enabled?: boolean;
  max_ttl_seconds?: number;
  default_ttl_seconds?: number;
  require_scope?: boolean;
  allowed_curves?: Curve[];
}

export interface FundingSettings {
  enabled?: boolean;
  providers?: string[];
  default_asset?: string;
}

export interface ComplianceSettings {
  terms_url?: string;
  privacy_url?: string;
  data_region?: string;
  retention_days?: number;
}

export interface AppDomain {
  id: string;
  origin: string;
  verified_at: string | null;
  created_at: string;
}

export interface Credential {
  id: string;
  kind: "app_secret" | "auth_key";
  label: string;
  last_four: string | null;
  public_key: string | null;
  key_hash: string | null;
  onchain_status: OnchainStatus;
  last_used_at: string | null;
  created_at: string;
  /** Present only in the response that creates it. */
  secret?: string;
}

export interface Issuer {
  id: string;
  issuer: string;
  issuer_hash: string | null;
  client_ids: string[];
  provider: string;
  label: string;
  enabled: boolean;
  onchain_status: OnchainStatus;
  created_at: string;
}

export interface NodeHealth {
  online: boolean;
  latency_ms: number | null;
  peer_count: number | null;
  uptime_pct_24h: number | null;
  last_error: string | null;
  observed_at: string | null;
}

export interface NodeOperator {
  address: string;
  name: string;
  slug: string;
  description: string;
  website_url: string | null;
  logo_url: string | null;
  api_url: string | null;
  region: string;
  jurisdiction: string;
  category: "enterprise" | "infrastructure" | "custodian" | "independent" | "signet";
  verified: boolean;
  is_open: boolean | null;
  registered_at: string | null;
  operator_address: string | null;
  group_count: number;
  synced_at: string | null;
  health: NodeHealth | null;
}

export interface GroupNode {
  address: string;
  status: "active" | "pending" | "removing";
  execute_after: string | null;
  removal_initiator: string | null;
  joined_at: string | null;
  operator: NodeOperator | null;
}

export interface OnchainIssuer {
  issuer: string;
  client_ids: string[];
}

export interface OnchainRemoval {
  node: string;
  execute_after: number;
  initiator: string;
}

export interface OnchainGroupState {
  address: string;
  manager: string;
  threshold: number;
  removal_delay: number;
  is_operational: boolean;
  active_nodes: string[];
  pending_nodes: string[];
  pending_removals: OnchainRemoval[];
  issuers: OnchainIssuer[];
  auth_keys: string[];
}

export interface GroupView {
  app: App;
  nodes: GroupNode[];
  onchain: OnchainGroupState | null;
  sync_error?: string;
}

export interface Key {
  id: string;
  key_id: string;
  curve: Curve;
  public_key: string | null;
  address: string | null;
  scope_hex: string | null;
  scope_kind: ScopeKind;
  scope_chain_id: number | null;
  scope_contract: string | null;
  scope_type_hash: string | null;
  parent_key_id: string | null;
  threshold: number | null;
  parties: string[];
  status: "enabled" | "disabled" | "deleted";
  label: string;
  subject_hash: string | null;
  last_used_at: string | null;
  synced_at: string;
  created_at: string;
}

export interface KeyStats {
  total: number;
  enabled: number;
  disabled: number;
  by_curve: Record<string, number>;
  by_scope: Record<string, number>;
}

export interface AppUser {
  id: string;
  subject_hash: string;
  issuer: string;
  label: string;
  key_count: number;
  status: "active" | "suspended";
  first_seen_at: string;
  last_seen_at: string;
}

export interface Delegation {
  id: string;
  key_id: string;
  parent_key_id: string;
  curve: Curve;
  label: string;
  subject_hash: string | null;
  issued_at: string;
  expires_at: string;
  revoked_at: string | null;
}

export type PolicyKind = "key_scope" | "spend_limit" | "allowlist" | "denylist" | "rate_limit";
export type EnforcedBy = "node" | "smart_account" | "advisory";

export interface Policy {
  id: string;
  name: string;
  kind: PolicyKind;
  enforced_by: EnforcedBy;
  config: Record<string, unknown>;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface Webhook {
  id: string;
  url: string;
  events: string[];
  enabled: boolean;
  description: string;
  created_at: string;
  secret?: string;
}

export interface WebhookDelivery {
  id: string;
  event: string;
  payload: unknown;
  status_code: number | null;
  error: string | null;
  attempt: number;
  duration_ms: number | null;
  created_at: string;
}

export interface UsagePoint {
  day: string;
  active_wallets: number;
  auth_count: number;
  keygen_count: number;
  sign_count: number;
  error_count: number;
  p50_latency_ms: number | null;
  p95_latency_ms: number | null;
}

export interface UsageSummary {
  from: string;
  to: string;
  monthly_active_wallets: number;
  total_signs: number;
  total_keygens: number;
  total_auths: number;
  error_rate: number;
  series: UsagePoint[];
}

export interface BillingAccount {
  currency: string;
  balance_micros: number;
  rate_micros_per_maw: number;
  low_balance_micros: number;
  contract_address: string | null;
  chain_id: number | null;
  status: "inactive" | "active" | "warning" | "suspended";
  payments_enabled: boolean;
  estimated_month_maw: number;
  estimated_month_cost_micros: number;
  updated_at: string;
}

export interface Invoice {
  id: string;
  period_start: string;
  period_end: string;
  active_wallets: number;
  rate_micros_per_maw: number;
  amount_micros: number;
  status: "draft" | "issued" | "paid" | "void";
  issued_at: string | null;
  paid_at: string | null;
}

export interface Asset {
  id: string;
  app_id: string | null;
  kind: string;
  scope: string;
  url: string;
  content_type: string;
  byte_size: number;
  created_at: string;
}

export interface AuditEntry {
  id: number;
  app_id: string | null;
  actor_label: string;
  action: string;
  target: string;
  metadata: Record<string, unknown>;
  created_at: string;
}

export interface NetworkStatus {
  chain_id: number;
  factory_address: string;
  chain_reachable: boolean;
  chain_error?: string;
  registered_nodes: number;
  groups: number;
  operators_listed: number;
  operators_online: number;
  checked_at: string;
}

/** A platform-signed auth-key certificate, and where to present it.
 *
 * The nodes accept this in place of an OAuth proof because the group trusts the
 * platform's authorization key on-chain — which is what lets a wallet sign-in
 * end with the developer holding a Signet key. */
export interface NodeCertificate {
  certificate: {
    identity: string;
    expiry: number;
    auth_key_pub: string;
    signature: string;
  };
  group_id: string;
  node_urls: string[];
  identity: string;
}
