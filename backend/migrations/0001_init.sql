-- Signet Platform — initial schema.
--
-- The platform stores *metadata and assets*. Authoritative state for signing
-- groups (membership, threshold, issuers, auth keys) lives on-chain in
-- SignetFactory/SignetGroup, and key material lives in the node KMS. Every
-- table below that mirrors chain or node state is a cache: it carries a
-- `synced_at` and is refreshed from the source, never treated as truth.

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

-- ─────────────────────────── Identity ───────────────────────────

-- A developer account. `subject` is the stable identifier from whichever
-- login route was used: the Signet account address (dogfooded FROST login) or
-- the EOA address (SIWE). Both are lower-cased hex.
CREATE TABLE users (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subject             TEXT NOT NULL UNIQUE,
    subject_kind        TEXT NOT NULL CHECK (subject_kind IN ('signet', 'siwe')),
    -- Counterfactual SignetAccount address (signet route) or the EOA (siwe).
    account_address     TEXT,
    -- Compressed group public key the SignetAccount is bound to (signet route).
    group_public_key    TEXT,
    email               CITEXT,
    display_name        TEXT NOT NULL DEFAULT '',
    avatar_url          TEXT,
    -- Platform staff may curate the node-operator marketplace.
    is_staff            BOOLEAN NOT NULL DEFAULT false,
    last_seen_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ─────────────────────────── Organizations ───────────────────────────

CREATE TABLE organizations (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT NOT NULL,
    slug                TEXT NOT NULL UNIQUE,
    logo_url            TEXT,
    website_url         TEXT,
    billing_email       CITEXT,
    plan                TEXT NOT NULL DEFAULT 'developer'
                        CHECK (plan IN ('developer', 'team', 'enterprise')),
    created_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE org_members (
    org_id              UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role                TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'developer', 'viewer')),
    invited_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id)
);
CREATE INDEX org_members_user_idx ON org_members(user_id);

CREATE TABLE org_invites (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email               CITEXT NOT NULL,
    role                TEXT NOT NULL CHECK (role IN ('admin', 'developer', 'viewer')),
    token_hash          TEXT NOT NULL UNIQUE,
    invited_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    accepted_at         TIMESTAMPTZ,
    accepted_by         UUID REFERENCES users(id) ON DELETE SET NULL,
    revoked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX org_invites_org_idx ON org_invites(org_id) WHERE accepted_at IS NULL AND revoked_at IS NULL;

-- ─────────────────────────── Apps ───────────────────────────

-- An app is the developer-facing unit. It is backed by exactly one on-chain
-- SignetGroup once deployed; before deployment `group_address` is NULL and the
-- app sits in the `provisioning` state.
CREATE TABLE apps (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    slug                TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    environment         TEXT NOT NULL DEFAULT 'development'
                        CHECK (environment IN ('development', 'staging', 'production')),
    status              TEXT NOT NULL DEFAULT 'provisioning'
                        CHECK (status IN ('provisioning', 'live', 'suspended', 'archived')),
    chain_id            BIGINT NOT NULL DEFAULT 31337,
    group_address       TEXT,
    group_public_key    TEXT,
    -- Threshold/quorum mirrored from the group contract for list rendering.
    threshold           INTEGER,
    node_count          INTEGER,
    is_operational      BOOLEAN,
    logo_url            TEXT,
    website_url         TEXT,
    created_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    deployed_at         TIMESTAMPTZ,
    archived_at         TIMESTAMPTZ,
    synced_at           TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, slug)
);
CREATE INDEX apps_org_idx ON apps(org_id) WHERE archived_at IS NULL;
CREATE UNIQUE INDEX apps_group_address_idx ON apps(lower(group_address)) WHERE group_address IS NOT NULL;

-- One settings row per app. Each column is a JSON document owned by one
-- configuration screen, so a partial save never clobbers a sibling section.
CREATE TABLE app_settings (
    app_id              UUID PRIMARY KEY REFERENCES apps(id) ON DELETE CASCADE,
    login_methods       JSONB NOT NULL DEFAULT '{}'::jsonb,
    branding            JSONB NOT NULL DEFAULT '{}'::jsonb,
    embedded_wallets    JSONB NOT NULL DEFAULT '{}'::jsonb,
    smart_accounts      JSONB NOT NULL DEFAULT '{}'::jsonb,
    session_signers     JSONB NOT NULL DEFAULT '{}'::jsonb,
    funding             JSONB NOT NULL DEFAULT '{}'::jsonb,
    compliance          JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Allowed browser origins for the client SDK (Privy's "Domains").
CREATE TABLE app_domains (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id              UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    origin              TEXT NOT NULL,
    verified_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (app_id, origin)
);

-- App credentials. `app_secret` is a platform-issued bearer secret (stored as
-- a hash, shown once). `auth_key` mirrors an on-chain secp256k1 authorization
-- key — the platform stores only its public half, never the private key.
CREATE TABLE app_credentials (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id              UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    kind                TEXT NOT NULL CHECK (kind IN ('app_secret', 'auth_key')),
    label               TEXT NOT NULL DEFAULT '',
    -- app_secret: bcrypt-style hash of the secret; auth_key: NULL.
    secret_hash         TEXT,
    -- Last four characters, so the console can identify a secret it can't show.
    last_four           TEXT,
    -- auth_key: compressed secp256k1 public key (hex) and keccak256 hash.
    public_key          TEXT,
    key_hash            TEXT,
    -- Mirrors on-chain presence for auth keys.
    onchain_status      TEXT NOT NULL DEFAULT 'unknown'
                        CHECK (onchain_status IN ('unknown', 'pending', 'active', 'removed')),
    created_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    last_used_at        TIMESTAMPTZ,
    revoked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX app_credentials_app_idx ON app_credentials(app_id) WHERE revoked_at IS NULL;

-- Trusted OAuth/OIDC issuers. Authoritative copy is on the group contract;
-- this table adds the display metadata the chain has no room for.
CREATE TABLE app_issuers (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id              UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    issuer              TEXT NOT NULL,
    issuer_hash         TEXT,
    client_ids          TEXT[] NOT NULL DEFAULT '{}',
    -- 'google' | 'apple' | 'custom' | … — drives the icon in the console.
    provider            TEXT NOT NULL DEFAULT 'custom',
    label               TEXT NOT NULL DEFAULT '',
    enabled             BOOLEAN NOT NULL DEFAULT true,
    onchain_status      TEXT NOT NULL DEFAULT 'unknown'
                        CHECK (onchain_status IN ('unknown', 'pending', 'active', 'removed')),
    synced_at           TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (app_id, issuer)
);

-- ─────────────────────────── Node operators ───────────────────────────

-- Off-chain operator directory. Replaces signet-ui's static
-- public/node-registry.json with a curated, queryable registry.
CREATE TABLE node_operators (
    address             TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    slug                TEXT NOT NULL UNIQUE,
    description         TEXT NOT NULL DEFAULT '',
    website_url         TEXT,
    logo_url            TEXT,
    api_url             TEXT,
    region              TEXT NOT NULL DEFAULT '',
    jurisdiction        TEXT NOT NULL DEFAULT '',
    category            TEXT NOT NULL DEFAULT 'independent'
                        CHECK (category IN ('enterprise', 'infrastructure', 'custodian', 'independent', 'signet')),
    contact_email       CITEXT,
    -- Curated by platform staff; unverified operators still list, flagged.
    verified            BOOLEAN NOT NULL DEFAULT false,
    -- Mirrored from SignetFactory.getNode().
    is_open             BOOLEAN,
    registered_at       TIMESTAMPTZ,
    operator_address    TEXT,
    group_count         INTEGER NOT NULL DEFAULT 0,
    synced_at           TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Rolling health probe results, used for the marketplace uptime figures.
CREATE TABLE node_health_samples (
    id                  BIGSERIAL PRIMARY KEY,
    node_address        TEXT NOT NULL REFERENCES node_operators(address) ON DELETE CASCADE,
    ok                  BOOLEAN NOT NULL,
    latency_ms          INTEGER,
    peer_count          INTEGER,
    error               TEXT,
    observed_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX node_health_recent_idx ON node_health_samples(node_address, observed_at DESC);

-- Cached group membership for an app, including timelocked removals.
CREATE TABLE app_nodes (
    app_id              UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    node_address        TEXT NOT NULL,
    status              TEXT NOT NULL CHECK (status IN ('active', 'pending', 'removing')),
    -- Unix seconds after which a queued removal may be executed.
    execute_after       TIMESTAMPTZ,
    removal_initiator   TEXT,
    joined_at           TIMESTAMPTZ,
    synced_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (app_id, node_address)
);

-- ─────────────────────────── Keys and end users ───────────────────────────

-- Key inventory mirrored from POST /admin/keys on a group node.
CREATE TABLE app_keys (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id              UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    key_id              TEXT NOT NULL,
    curve               TEXT NOT NULL
                        CHECK (curve IN ('frost_secp256k1', 'frost_ed25519', 'ecdsa_secp256k1')),
    public_key          TEXT,
    address             TEXT,
    -- Scope bytes per DESIGN-SCOPED-SUBKEYS (0x00 unscoped, 0x01 UserOp,
    -- 0x02 Solana, 0x03 EIP-712 domain+type).
    scope_hex           TEXT,
    scope_kind          TEXT NOT NULL DEFAULT 'unscoped'
                        CHECK (scope_kind IN ('unscoped', 'evm_userop', 'solana_tx', 'eip712')),
    scope_chain_id      BIGINT,
    scope_contract      TEXT,
    scope_type_hash     TEXT,
    parent_key_id       TEXT,
    threshold           INTEGER,
    parties             TEXT[] NOT NULL DEFAULT '{}',
    status              TEXT NOT NULL DEFAULT 'enabled'
                        CHECK (status IN ('enabled', 'disabled', 'deleted')),
    label               TEXT NOT NULL DEFAULT '',
    -- Hash of the end-user subject this key belongs to, when derivable.
    subject_hash        TEXT,
    last_used_at        TIMESTAMPTZ,
    synced_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (app_id, key_id, curve)
);
CREATE INDEX app_keys_app_idx ON app_keys(app_id);
CREATE INDEX app_keys_subject_idx ON app_keys(app_id, subject_hash);

-- End users of an app, identified only by a hash of (iss, sub). The platform
-- never sees the raw OAuth subject — nodes hash it and the metering feed
-- carries the hash — so this table cannot deanonymize an app's users.
CREATE TABLE app_users (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id              UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    subject_hash        TEXT NOT NULL,
    issuer              TEXT NOT NULL DEFAULT '',
    -- Optional developer-supplied label; never populated by the protocol.
    label               TEXT NOT NULL DEFAULT '',
    key_count           INTEGER NOT NULL DEFAULT 0,
    status              TEXT NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'suspended')),
    first_seen_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (app_id, subject_hash)
);
CREATE INDEX app_users_last_seen_idx ON app_users(app_id, last_seen_at DESC);

-- Delegation tokens minted for agents (signet-sdk requestDelegation). The
-- platform records the metadata so a developer can see and revoke them; the
-- token itself is never stored.
CREATE TABLE delegations (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id              UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    key_id              TEXT NOT NULL,
    parent_key_id       TEXT NOT NULL DEFAULT '',
    curve               TEXT NOT NULL DEFAULT 'ecdsa_secp256k1',
    label               TEXT NOT NULL DEFAULT '',
    subject_hash        TEXT,
    -- SHA-256 of the token, so a presented token can be matched to this row
    -- without the platform holding anything usable.
    token_hash          TEXT,
    issued_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,
    created_by          UUID REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX delegations_app_idx ON delegations(app_id) WHERE revoked_at IS NULL;

-- Developer-defined policies. `kind` selects the config shape; `enforced_by`
-- records where the rule actually bites, which is the honest distinction
-- between a scope (enforced by every node) and a spend cap (today advisory).
CREATE TABLE policies (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id              UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    kind                TEXT NOT NULL CHECK (kind IN ('key_scope', 'spend_limit', 'allowlist', 'denylist', 'rate_limit')),
    enforced_by         TEXT NOT NULL DEFAULT 'advisory'
                        CHECK (enforced_by IN ('node', 'smart_account', 'advisory')),
    config              JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled             BOOLEAN NOT NULL DEFAULT true,
    created_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (app_id, name)
);

-- ─────────────────────────── Webhooks ───────────────────────────

CREATE TABLE webhooks (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id              UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    url                 TEXT NOT NULL,
    -- HMAC-SHA256 signing secret, shown once at creation.
    secret              TEXT NOT NULL,
    events              TEXT[] NOT NULL DEFAULT '{}',
    enabled             BOOLEAN NOT NULL DEFAULT true,
    description         TEXT NOT NULL DEFAULT '',
    created_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE webhook_deliveries (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id          UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event               TEXT NOT NULL,
    payload             JSONB NOT NULL,
    status_code         INTEGER,
    error               TEXT,
    attempt             INTEGER NOT NULL DEFAULT 1,
    duration_ms         INTEGER,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX webhook_deliveries_hook_idx ON webhook_deliveries(webhook_id, created_at DESC);

-- ─────────────────────────── Usage and billing ───────────────────────────

-- Raw metering events, fed by the node fleet (or the platform's own proxy).
CREATE TABLE usage_events (
    id                  BIGSERIAL PRIMARY KEY,
    app_id              UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    subject_hash        TEXT,
    kind                TEXT NOT NULL CHECK (kind IN ('auth', 'keygen', 'sign', 'delegate', 'reshare')),
    curve               TEXT,
    key_id              TEXT,
    node_address        TEXT,
    latency_ms          INTEGER,
    ok                  BOOLEAN NOT NULL DEFAULT true,
    error_code          TEXT,
    occurred_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX usage_events_app_time_idx ON usage_events(app_id, occurred_at DESC);

-- Daily rollup. Monthly active wallets — the billing unit — is a distinct
-- count of subject_hash over the settlement period, so it cannot be summed
-- from these rows; it is recomputed from usage_events at settlement time.
CREATE TABLE usage_daily (
    app_id              UUID NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    day                 DATE NOT NULL,
    active_wallets      INTEGER NOT NULL DEFAULT 0,
    auth_count          INTEGER NOT NULL DEFAULT 0,
    keygen_count        INTEGER NOT NULL DEFAULT 0,
    sign_count          INTEGER NOT NULL DEFAULT 0,
    error_count         INTEGER NOT NULL DEFAULT 0,
    p50_latency_ms      INTEGER,
    p95_latency_ms      INTEGER,
    PRIMARY KEY (app_id, day)
);

-- Billing is scaffolded but inert until payments are switched on: the balance
-- is denominated in USDC micros to match the on-chain BillingRegistry, and no
-- code path charges it yet.
CREATE TABLE billing_accounts (
    org_id                  UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    currency                TEXT NOT NULL DEFAULT 'USDC',
    balance_micros          BIGINT NOT NULL DEFAULT 0,
    -- Per-MAW rate in USDC micros; 50000 = $0.05, the documented launch rate.
    rate_micros_per_maw     BIGINT NOT NULL DEFAULT 50000,
    low_balance_micros      BIGINT NOT NULL DEFAULT 5000000,
    contract_address        TEXT,
    chain_id                BIGINT,
    status                  TEXT NOT NULL DEFAULT 'inactive'
                            CHECK (status IN ('inactive', 'active', 'warning', 'suspended')),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE billing_transactions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    kind                TEXT NOT NULL CHECK (kind IN ('topup', 'charge', 'refund', 'credit')),
    amount_micros       BIGINT NOT NULL,
    tx_hash             TEXT,
    chain_id            BIGINT,
    memo                TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX billing_transactions_org_idx ON billing_transactions(org_id, created_at DESC);

CREATE TABLE invoices (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    period_start        DATE NOT NULL,
    period_end          DATE NOT NULL,
    active_wallets      INTEGER NOT NULL DEFAULT 0,
    rate_micros_per_maw BIGINT NOT NULL,
    amount_micros       BIGINT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'draft'
                        CHECK (status IN ('draft', 'issued', 'paid', 'void')),
    issued_at           TIMESTAMPTZ,
    paid_at             TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, period_start)
);

-- ─────────────────────────── Assets and audit ───────────────────────────

-- Every upload the platform stores, so an org can see (and reclaim) what it
-- holds. `object_key` is the storage key; `url` is what clients render.
CREATE TABLE assets (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID REFERENCES organizations(id) ON DELETE CASCADE,
    app_id              UUID REFERENCES apps(id) ON DELETE CASCADE,
    kind                TEXT NOT NULL DEFAULT 'image'
                        CHECK (kind IN ('image', 'logo', 'document', 'other')),
    scope               TEXT NOT NULL DEFAULT 'misc',
    object_key          TEXT NOT NULL UNIQUE,
    url                 TEXT NOT NULL,
    content_type        TEXT NOT NULL DEFAULT 'application/octet-stream',
    byte_size           BIGINT NOT NULL DEFAULT 0,
    uploaded_by         UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX assets_org_idx ON assets(org_id, created_at DESC);

CREATE TABLE audit_log (
    id                  BIGSERIAL PRIMARY KEY,
    org_id              UUID REFERENCES organizations(id) ON DELETE CASCADE,
    app_id              UUID REFERENCES apps(id) ON DELETE CASCADE,
    actor_user_id       UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_label         TEXT NOT NULL DEFAULT '',
    action              TEXT NOT NULL,
    target              TEXT NOT NULL DEFAULT '',
    metadata            JSONB NOT NULL DEFAULT '{}'::jsonb,
    ip                  TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_org_idx ON audit_log(org_id, created_at DESC);
CREATE INDEX audit_log_app_idx ON audit_log(app_id, created_at DESC);

-- Login challenges. Single-use, short-lived nonces for the FROST and SIWE
-- login routes; consumed rows are kept briefly so a replay is diagnosable.
CREATE TABLE auth_challenges (
    nonce               TEXT PRIMARY KEY,
    method              TEXT NOT NULL CHECK (method IN ('signet', 'siwe')),
    address             TEXT,
    statement           TEXT NOT NULL DEFAULT '',
    consumed_at         TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX auth_challenges_expiry_idx ON auth_challenges(expires_at);
