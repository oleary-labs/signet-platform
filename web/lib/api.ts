"use client";

import type {
  App,
  AppDomain,
  AppSettings,
  AppUser,
  Asset,
  AuditEntry,
  BillingAccount,
  Credential,
  Delegation,
  EnvironmentCheck,
  GroupNode,
  GroupView,
  Invoice,
  Issuer,
  Key,
  KeyStats,
  NetworkConfig,
  NetworkStatus,
  NodeCertificate,
  NodeOperator,
  OrgInvite,
  OrgMember,
  Organization,
  Policy,
  Session,
  UsageSummary,
  User,
  Webhook,
  WebhookDelivery,
} from "./types";
import type { SignedUserOp } from "./userop";

export const API_BASE =
  process.env.NEXT_PUBLIC_API_URL?.replace(/\/$/, "") || "http://localhost:8080";

export class ApiError extends Error {
  status: number;
  body: unknown;
  /** Present when the server refused an environment change; carries which
   *  requirements are unmet so the caller can show them rather than a
   *  bare failure. */
  check?: EnvironmentCheck;
  constructor(status: number, body: unknown) {
    const message =
      (body && typeof body === "object" && "error" in body
        ? String((body as { error: unknown }).error)
        : null) || `request failed (${status})`;
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
    if (body && typeof body === "object" && "check" in body) {
      this.check = (body as { check: EnvironmentCheck }).check;
    }
  }
}

/**
 * The session token.
 *
 * The server also sets an HttpOnly cookie, which is what actually authorizes
 * same-site requests. This copy exists so the console can send an Authorization
 * header when the API is on a different origin — a cookie would not be sent
 * there — and is cleared on sign-out.
 */
const TOKEN_KEY = "signet_platform_token";

export function storedToken(): string | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage.getItem(TOKEN_KEY);
  } catch {
    // Private-mode browsers throw on storage access; the cookie still works.
    return null;
  }
}

export function storeToken(token: string | null) {
  if (typeof window === "undefined") return;
  try {
    if (token) window.localStorage.setItem(TOKEN_KEY, token);
    else window.localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* ignore — the cookie is the primary credential */
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const token = storedToken();
  const res = await fetch(`${API_BASE}${path}`, {
    method,
    credentials: "include",
    headers: {
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  const json = text ? safeParse(text) : null;
  if (!res.ok) throw new ApiError(res.status, json ?? text);
  return json as T;
}

function safeParse(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

const get = <T,>(path: string) => request<T>("GET", path);
const post = <T,>(path: string, body?: unknown) => request<T>("POST", path, body);
const patch = <T,>(path: string, body?: unknown) => request<T>("PATCH", path, body);
const put = <T,>(path: string, body?: unknown) => request<T>("PUT", path, body);
const del = <T,>(path: string, body?: unknown) => request<T>("DELETE", path, body);

export interface Challenge {
  nonce: string;
  method: "signet" | "siwe";
  statement: string;
  expires_at: string;
  message?: string;
  digest?: string;
  domain?: string;
  chain_id?: number;
  uri?: string;
  issued_at?: string;
}

export interface VerifyResult {
  session: Session;
  token: string;
  expires_at: string;
}

export const api = {
  // ── Public ────────────────────────────────────────────────────────────
  config: () => get<NetworkConfig>("/v1/config"),
  status: () => get<NetworkStatus>("/v1/status"),
  webhookEvents: () => get<{ events: string[] }>("/v1/webhooks/events"),
  nodeOperators: (params?: Record<string, string | boolean | undefined>) =>
    get<NodeOperator[]>(`/v1/marketplace/nodes${queryString(params)}`),
  nodeOperator: (address: string) =>
    get<NodeOperator>(`/v1/marketplace/nodes/${encodeURIComponent(address)}`),

  // ── Auth ──────────────────────────────────────────────────────────────
  challenge: (method: "signet" | "siwe", address?: string) =>
    post<Challenge>("/v1/auth/challenge", { method, address }),
  verify: (body: Record<string, unknown>) => post<VerifyResult>("/v1/auth/verify", body),
  logout: () => post<{ status: string }>("/v1/auth/logout"),
  /** A platform-signed certificate the nodes accept in place of an OAuth
   *  proof, so a wallet sign-in still yields a Signet key. */
  nodeCertificate: (session_pub: string) =>
    post<NodeCertificate>("/v1/auth/node-certificate", { session_pub }),
  recordSignetKey: (body: Record<string, unknown>) => put<User>("/v1/me/signet-key", body),
  me: () => get<Session>("/v1/me"),
  updateMe: (body: Partial<Pick<User, "display_name" | "email" | "avatar_url">>) =>
    patch<User>("/v1/me", body),
  myApps: () => get<App[]>("/v1/me/apps"),
  acceptInvite: (token: string) =>
    post<Organization>(`/v1/invites/${encodeURIComponent(token)}/accept`),

  // ── Organizations ─────────────────────────────────────────────────────
  orgs: () => get<Organization[]>("/v1/orgs"),
  createOrg: (name: string, billing_email?: string) =>
    post<Organization>("/v1/orgs", { name, billing_email }),
  org: (orgId: string) => get<Organization>(`/v1/orgs/${orgId}`),
  updateOrg: (orgId: string, body: Record<string, unknown>) =>
    patch<Organization>(`/v1/orgs/${orgId}`, body),
  members: (orgId: string) => get<OrgMember[]>(`/v1/orgs/${orgId}/members`),
  setMemberRole: (orgId: string, userId: string, role: string) =>
    patch<{ status: string }>(`/v1/orgs/${orgId}/members/${userId}`, { role }),
  removeMember: (orgId: string, userId: string) =>
    del<{ status: string }>(`/v1/orgs/${orgId}/members/${userId}`),
  invites: (orgId: string) => get<OrgInvite[]>(`/v1/orgs/${orgId}/invites`),
  createInvite: (orgId: string, email: string, role: string) =>
    post<OrgInvite>(`/v1/orgs/${orgId}/invites`, { email, role }),
  revokeInvite: (orgId: string, inviteId: string) =>
    del<{ status: string }>(`/v1/orgs/${orgId}/invites/${inviteId}`),
  orgAudit: (orgId: string) => get<AuditEntry[]>(`/v1/orgs/${orgId}/audit`),
  assets: (orgId: string) => get<Asset[]>(`/v1/orgs/${orgId}/assets`),
  deleteAsset: (orgId: string, assetId: string) =>
    del<{ status: string }>(`/v1/orgs/${orgId}/assets/${assetId}`),

  // ── Billing ───────────────────────────────────────────────────────────
  billing: (orgId: string) =>
    get<{ account: BillingAccount; note: string }>(`/v1/orgs/${orgId}/billing`),
  updateBilling: (orgId: string, body: Record<string, unknown>) =>
    patch<BillingAccount>(`/v1/orgs/${orgId}/billing`, body),
  invoices: (orgId: string) => get<Invoice[]>(`/v1/orgs/${orgId}/invoices`),
  draftInvoice: (orgId: string, body?: Record<string, unknown>) =>
    post<{ invoice: Invoice; note: string }>(`/v1/orgs/${orgId}/invoices/draft`, body ?? {}),

  // ── Apps ──────────────────────────────────────────────────────────────
  apps: (orgId: string) => get<App[]>(`/v1/orgs/${orgId}/apps`),
  createApp: (orgId: string, body: Record<string, unknown>) =>
    post<App>(`/v1/orgs/${orgId}/apps`, body),
  app: (appId: string) => get<App>(`/v1/apps/${appId}`),
  updateApp: (appId: string, body: Record<string, unknown>) =>
    patch<App>(`/v1/apps/${appId}`, body),
  archiveApp: (appId: string) =>
    del<{ status: string; note: string }>(`/v1/apps/${appId}`),

  environmentCheck: (appId: string, to: string) =>
    get<EnvironmentCheck>(`/v1/apps/${appId}/environment/check?to=${to}`),

  settings: (appId: string) => get<AppSettings>(`/v1/apps/${appId}/settings`),
  saveSettings: (appId: string, section: keyof AppSettings, doc: unknown) =>
    put<AppSettings>(`/v1/apps/${appId}/settings/${section}`, doc),

  domains: (appId: string) => get<AppDomain[]>(`/v1/apps/${appId}/domains`),
  addDomain: (appId: string, origin: string) =>
    post<AppDomain>(`/v1/apps/${appId}/domains`, { origin }),
  removeDomain: (appId: string, domainId: string) =>
    del<{ status: string }>(`/v1/apps/${appId}/domains/${domainId}`),

  credentials: (appId: string) => get<Credential[]>(`/v1/apps/${appId}/credentials`),
  createCredential: (appId: string, body: Record<string, unknown>) =>
    post<Credential>(`/v1/apps/${appId}/credentials`, body),
  /** Revoking an authorization key removes it on-chain too, so a signed
   *  operation is required whenever the key is actually registered. */
  revokeCredential: (appId: string, credentialId: string, user_op?: SignedUserOp) =>
    del<{ status: string; transaction_hash: string }>(
      `/v1/apps/${appId}/credentials/${credentialId}`,
      user_op ? { user_op } : undefined,
    ),

  issuers: (appId: string) => get<Issuer[]>(`/v1/apps/${appId}/issuers`),
  saveIssuer: (appId: string, body: Record<string, unknown>) =>
    put<{ issuer: Issuer; transaction_hash: string }>(`/v1/apps/${appId}/issuers`, body),
  removeIssuer: (appId: string, issuerId: string, user_op?: SignedUserOp) =>
    del<{ status: string; transaction_hash: string }>(
      `/v1/apps/${appId}/issuers/${issuerId}`,
      user_op ? { user_op } : undefined,
    ),

  group: (appId: string) => get<GroupView>(`/v1/apps/${appId}/group`),
  /** Create a development group. The console signs the operation; the platform
   *  checks it and forwards it to the bundler, and the paymaster pays. The
   *  developer's smart wallet is the caller, so it is the manager from the
   *  first block. */
  deployGroup: (appId: string, body: Record<string, unknown>) =>
    post<{
      app: App;
      group_address: string;
      manager: string;
      transaction_hash: string;
    }>(`/v1/apps/${appId}/group/deploy`, body),

  /** Membership, reshare, and auth-resolver changes on an existing group. */
  executeGroupCall: (appId: string, user_op: SignedUserOp, action: string) =>
    post<{ app: App; nodes: GroupNode[]; transaction_hash: string; sync_error?: string }>(
      `/v1/apps/${appId}/group/execute`,
      { user_op, action },
    ),

  attachGroup: (appId: string, group_address: string, group_public_key?: string) =>
    post<{ app: App; sync_error?: string }>(`/v1/apps/${appId}/group/attach`, {
      group_address,
      group_public_key,
    }),
  syncGroup: (appId: string) => post<GroupView>(`/v1/apps/${appId}/group/sync`),

  keys: (appId: string, params?: Record<string, string | undefined>) =>
    get<{ keys: Key[]; stats: KeyStats }>(`/v1/apps/${appId}/keys${queryString(params)}`),
  syncKeys: (appId: string, keys: unknown[]) =>
    post<{ synced: number }>(`/v1/apps/${appId}/keys/sync`, { keys }),
  updateKey: (appId: string, curve: string, keyId: string, body: Record<string, unknown>) =>
    patch<Key>(`/v1/apps/${appId}/keys/${curve}/${encodeURIComponent(keyId)}`, body),

  appUsers: (appId: string, params?: Record<string, string | undefined>) =>
    get<AppUser[]>(`/v1/apps/${appId}/users${queryString(params)}`),
  labelAppUser: (appId: string, userId: string, label: string) =>
    patch<{ status: string }>(`/v1/apps/${appId}/users/${userId}`, { label }),

  delegations: (appId: string, includeRevoked = false) =>
    get<Delegation[]>(
      `/v1/apps/${appId}/delegations${includeRevoked ? "?include_revoked=true" : ""}`,
    ),
  recordDelegation: (appId: string, body: Record<string, unknown>) =>
    post<Delegation>(`/v1/apps/${appId}/delegations`, body),
  revokeDelegation: (appId: string, delegationId: string) =>
    del<{ status: string; note: string }>(`/v1/apps/${appId}/delegations/${delegationId}`),

  policies: (appId: string) => get<Policy[]>(`/v1/apps/${appId}/policies`),
  savePolicy: (appId: string, body: Record<string, unknown>) =>
    put<Policy>(`/v1/apps/${appId}/policies`, body),
  deletePolicy: (appId: string, policyId: string) =>
    del<{ status: string }>(`/v1/apps/${appId}/policies/${policyId}`),

  webhooks: (appId: string) => get<Webhook[]>(`/v1/apps/${appId}/webhooks`),
  createWebhook: (appId: string, body: Record<string, unknown>) =>
    post<Webhook>(`/v1/apps/${appId}/webhooks`, body),
  updateWebhook: (appId: string, webhookId: string, body: Record<string, unknown>) =>
    patch<Webhook>(`/v1/apps/${appId}/webhooks/${webhookId}`, body),
  deleteWebhook: (appId: string, webhookId: string) =>
    del<{ status: string }>(`/v1/apps/${appId}/webhooks/${webhookId}`),
  deliveries: (appId: string, webhookId: string) =>
    get<WebhookDelivery[]>(`/v1/apps/${appId}/webhooks/${webhookId}/deliveries`),
  testWebhook: (appId: string, webhookId: string) =>
    post<{ delivered: boolean; note: string; error?: string }>(
      `/v1/apps/${appId}/webhooks/${webhookId}/test`,
    ),

  usage: (appId: string, params?: Record<string, string | undefined>) =>
    get<UsageSummary>(`/v1/apps/${appId}/usage${queryString(params)}`),
  appAudit: (appId: string) => get<AuditEntry[]>(`/v1/apps/${appId}/audit`),

  // ── Node proxy ────────────────────────────────────────────────────────
  /**
   * Forward a request to a signetd node.
   *
   * signetd sets no CORS headers, so the browser cannot call it directly. The
   * body is signed with the developer's own session or auth key before it
   * leaves this machine; the platform only relays it.
   */
  nodeProxy: async (nodeUrl: string, path: string, body?: unknown, method = "POST") => {
    const token = storedToken();
    const res = await fetch(`${API_BASE}/v1/node/proxy`, {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "x-node-url": nodeUrl,
        "x-node-path": path,
        "x-node-method": method,
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
    const text = await res.text();
    const json = text ? safeParse(text) : null;
    if (!res.ok) throw new ApiError(res.status, json ?? text);
    return json;
  },

  // ── Uploads ───────────────────────────────────────────────────────────
  presignUpload: (body: Record<string, unknown>) =>
    post<{ upload_url: string; public_url: string; object_key: string; asset: Asset }>(
      "/v1/uploads/presign",
      body,
    ),
};

/** Upload a file through a presigned PUT and return its public URL. */
export async function uploadFile(
  file: File,
  opts: { orgId: string; appId?: string; scope?: string },
): Promise<string> {
  const { upload_url, public_url } = await api.presignUpload({
    org_id: opts.orgId,
    app_id: opts.appId,
    filename: file.name,
    content_type: file.type || "application/octet-stream",
    scope: opts.scope ?? "misc",
    byte_size: file.size,
  });
  const put = await fetch(upload_url, {
    method: "PUT",
    headers: { "Content-Type": file.type || "application/octet-stream" },
    body: file,
  });
  if (!put.ok) throw new Error(`upload failed (${put.status})`);
  return public_url;
}

function queryString(params?: Record<string, string | boolean | undefined>): string {
  if (!params) return "";
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === "" || v === false) continue;
    q.set(k, String(v));
  }
  const s = q.toString();
  return s ? `?${s}` : "";
}
