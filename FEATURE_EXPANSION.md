# Node feature expansion required for Privy parity

**Status:** analysis, not a commitment. Nothing here has been implemented.
**Audience:** whoever owns `signet-protocol`.
**Written against:** `signet-protocol` @ the tree cloned 2026-08-31, `signet-sdk` 0.3.0.

---

## Why this document exists

The platform in this repository is feature-complete against `signet-ui` and is
shaped to match what a developer arriving from Privy expects to find. Building
it surfaced a precise list of things the console can *describe* but the node
cannot yet *enforce* — and a few it cannot even represent.

Those gaps are enumerated below. Each one says what Privy does, what Signet does
today, what is actually missing at the node layer, and a suggested
implementation path that fits the existing architecture rather than fighting it.

Two rules were applied throughout, and they are worth stating because they
decided several of the recommendations:

1. **The platform must never become a trusted party.** Any feature whose
   obvious implementation is "the platform checks it" is wrong. If a
   constraint matters, a threshold of independent operators has to enforce it,
   or the chain does.
2. **Say what is enforced where.** Where a gap cannot be closed soon, the
   console labels the feature honestly — `enforced_by: advisory` on a policy,
   a warning under a login method the nodes cannot verify. Shipping a
   convincing-looking toggle that enforces nothing would be worse than the gap.

Where the console already does this, the file and screen are named, so closing
a gap includes removing the caveat that stands in for it.

---

## Priority 0 — blocks the platform's core loop

### P0.1 Metered usage feed from the node fleet

| | |
|---|---|
| **Privy** | Meters monthly active wallets and bills on them. |
| **Signet today** | Nodes log to stdout. Nothing emits structured usage anywhere. |
| **Missing** | A producer for the metering feed the platform already consumes. |

The platform exposes `POST /v1/ingest/usage`
(`backend/internal/api/handlers_misc.go`), authenticated by a shared key, and
every analytics and billing screen reads from it. **It currently has no
producer.** Until a node emits these events, active-wallet counts are only
populated by the seed script, and billing cannot be switched on at all.

`docs/ECONOMIC-MODEL.md` §10 already anticipates this under "liveness proofs and
metering". This is the smallest change with the largest unblocking effect in
this document.

**Suggested path.** Add an optional `metering` block to the node config:

```yaml
metering:
  endpoint: https://api.signet.dev/v1/ingest/usage
  key_env:  SIGNET_METERING_KEY   # never the key itself, in config
  batch:    100
  flush_secs: 30
```

Emit one event per completed `/v1/auth`, `/v1/keygen`, `/v1/sign`, and
`/v1/delegate`, with the shape the platform already accepts:

```json
{ "group_address": "0x…", "subject_hash": "sha256(iss:sub)", "kind": "sign",
  "curve": "ecdsa_secp256k1", "key_id": "…", "node_address": "0x…",
  "latency_ms": 142, "ok": true, "occurred_at": 1788226656 }
```

Three properties matter and are cheap to get right at the start:

- **Hash the subject at the node.** `sha256(iss:sub)`, never the raw subject.
  The platform can then count distinct active wallets — which is the billing
  unit — without ever being able to deanonymize an application's users. The
  platform already assumes this (`app_users.subject_hash`) and never asks for
  more.
- **Buffer and drop, never block.** A metering sink that is down must not slow
  or fail a signing round. Bounded in-memory buffer, drop on overflow, count
  the drops.
- **Every node emits.** The platform de-duplicates on
  `(group, subject_hash, day)` for the wallet count, so a signing round
  reported by three participants counts once. Per-node reporting is what makes
  it possible to attribute the operator revenue share later, and to notice an
  operator that is quietly not participating.

**Effort:** small. **Unblocks:** analytics, billing, usage-threshold webhooks,
operator revenue attribution.

---

### P0.2 Publish `@oleary-labs/signet-circuits` at a version the SDK accepts

| | |
|---|---|
| **Signet today** | `signet-sdk@0.3.0` peer-requires `@oleary-labs/signet-circuits@^0.3.0`. Only `0.1.0` is published to npm. |
| **Missing** | A release, not a feature. |

Browser-side ZK proving cannot be installed from npm today. Anyone following
the documented client-proving path hits an unsatisfiable peer dependency, and
`@aztec/bb.js` is additionally pinned to an exact `0.82.2` that conflicts with
what `signet-ui` has installed.

This console therefore proves **server-side** through the bundler's
`/v1/prove` — see the comment in `web/app/auth/callback/page.tsx`. That is a
defensible choice for a first-party console, and the wrong default to force on
every integrator: client-side proving is the stronger trust model, and it is
the one the positioning rests on ("the credential never leaves the device").

**Suggested path.** Publish `signet-circuits` at a version matching the SDK's
range, relax the `bb.js` peer pin to a caret range, and add a CI job that
installs the SDK into an empty project with peers and builds it. A peer graph
that cannot be resolved from a clean registry is a release bug, and one that
only shows up for external integrators.

**Effort:** trivial. **Unblocks:** the strongest form of the product's central
claim.

---

### P0.4 Bundler submission authentication

*This one is in `signet-min-bundler`, not the node — but it is the last guard
missing from the sponsorship path, so it belongs with the P0 items.*

| | |
|---|---|
| **Privy** | Sponsorship is scoped to an app, with a server-side key and per-app policy. |
| **Signet today** | The bundler checks `X-API-Key` on the **prover** endpoint only. `eth_sendUserOperation` and `pm_getPaymasterData` are open to anyone who can reach the port. |
| **Missing** | The same header check on the JSON-RPC endpoint. |

Every on-chain action the console takes is a UserOperation the platform decodes
before forwarding (`backend/internal/userop`): the sender must be the caller's
own smart wallet, the call must be `execute(dest, 0, func)`, `dest` must be the
contract the route names, and the selector must be one that route allows. That
governs what the *platform* submits. It does not govern the bundler, which will
take an operation from anyone.

What still stands between that and drained sponsorship is `SignetPaymaster`
itself: `_validatePaymasterUserOp` requires a signature from the bundler's
`verifyingSigner`, and `_isAllowedTarget` restricts sponsored calls to the
factory, a factory-deployed group, or the sender itself. So an attacker cannot
sponsor arbitrary transactions — but they *can* sponsor group operations of
their own from any wallet, without ever going through the console. The
bundler's optional `sponsorGating` whitelist narrows that further, and the
platform cannot use it: senders are added by a CLI writing to the bundler's
database (`cmd/invite`), with no network path for the backend to whitelist a
smart wallet as it provisions one.

**Suggested path.** Two changes, in order of value:

1. Apply the existing `X-API-Key` middleware — the one in
   `internal/prover/handler.go` — to the JSON-RPC handler, gated on a new
   `rpcApiKey` config value. Empty keeps it open, so devnets are unaffected.
   The platform already sends the header on every call
   (`userop.NewClient(..., apiKey)`, wired from `BUNDLER_API_KEY`), so nothing
   changes on this side when it lands.
2. Give `sponsorGating` an authenticated admin route to whitelist a sender, so
   the platform can add each smart wallet at the moment it provisions the
   developer's Signet key. That turns sponsorship from "any group operation" to
   "operations from wallets this platform issued".

Until (1) ships, a public deployment should keep the bundler on a private
network and expose only the platform backend — which is what `boot.sh` does
locally by binding it to `127.0.0.1`.

**Effort:** small (1), moderate (2). **Unblocks:** running the sponsored path on
a public network at all.

---

## Priority 1 — login methods a developer will expect on day one

The console lists these under Configuration → Login methods, split into
"enforced by your operators" and "offered in your login modal", with a warning
that the second group needs an issuer the nodes can verify. Closing these gaps
means moving items across that line and deleting the warning.

### P1.1 Email and SMS one-time codes

| | |
|---|---|
| **Privy** | First-class. Often the majority of sign-ins. |
| **Signet today** | No route. The ZK circuit verifies OIDC ID tokens; an OTP is not one. |

**Suggested path — an issuer, not a node change.** Ship a reference OTP issuer
that mints RS256 / RSA-2048 ID tokens the existing circuit already accepts.
`signet-better-mcp` demonstrates the shape: Better Auth in-process, issuing
RS256 JWTs with OIDC discovery. Package that as `signet-issuer`, deployable by
an application, and register it as a trusted issuer on the group.

**The trade-off has to be stated plainly, not buried.** An application running
its own issuer becomes a trusted authority over its own users: compromise the
issuer and you can mint a token for any of them. That is strictly better than
today (nothing) and strictly worse than the Google path (where the trust anchor
is eliminated after proof generation). The console should label an
application-operated issuer differently from a public one, because a developer
choosing between them is choosing between two different security models and
deserves to know it.

**Alternative, stronger, much larger.** A dedicated circuit proving possession
of an OTP delivered to a channel bound at registration. Not worth it before
there is demand.

**Effort:** medium (a deployable issuer). **Node change:** none.

---

### P1.2 Passkeys / WebAuthn

| | |
|---|---|
| **Privy** | Passkey login and passkey MFA. |
| **Signet today** | No route. |

Passkeys are the strongest consumer authenticator available and they are
*already* a signature scheme — wrapping one in a JWT to fit the existing circuit
would be pure overhead.

**Suggested path — a native auth scheme, no circuit.** Add `POST /v1/auth`
scheme `webauthn`, structurally parallel to the existing SIWE scheme
(`node/siwe.go`):

- The group registers credential IDs and their P-256 public keys, either on the
  group contract next to the auth keys, or through an `ISignetAuthResolver`
  adapter — which already exists as the extension point for exactly this kind
  of thing.
- The client presents a WebAuthn assertion whose challenge is the session
  public key.
- Every node independently verifies the assertion: signature over
  `authenticatorData ‖ SHA-256(clientDataJSON)`, challenge equals the session
  key, origin and RP ID match the group's registered relying party, and the
  signature counter has not regressed.

P-256 verification is cheap and every node does it independently, so the
security property is the same as every other route: no single operator's
acceptance is load-bearing.

**Effort:** medium. **Node change:** a new auth scheme (~300 lines plus
storage), and a registry for credentials.

---

### P1.3 ES256 and EdDSA-signed ID tokens

| | |
|---|---|
| **Signet today** | The Noir circuit is RSA-2048 / RS256 only. |
| **Missing** | Circuit variants, or a non-ZK fallback. |

An increasing share of OIDC providers sign with ES256, and some enterprise IdPs
offer nothing else. Today those issuers simply cannot be used, which quietly
rules out a chunk of the enterprise segment the positioning targets.

**Suggested path.** Add an ES256 circuit variant in `signet-circuits`, with the
same public-input shape so the node's verification path is a switch on the
issuer's advertised algorithm rather than a second code path. Reuse the JWKS
cache. Note that the RSA-2048 circuit also excludes RSA-4096, which a few
providers use.

**Effort:** medium-large (circuit work, and a second verification key to embed
and audit).

---

### P1.4 Farcaster, Telegram, and other non-OIDC identities

| | |
|---|---|
| **Privy** | Farcaster, Telegram, and a long tail of social logins. |
| **Signet today** | No route. Neither is OIDC. |

Farcaster identity is an Ed25519 signature under an on-chain key registry;
Telegram is an HMAC over a bot token.

**Suggested path.** Farcaster fits the existing `ISignetAuthResolver`
interface almost exactly — an adapter resolving an address to an FID via the
key registry, with the group binding it as its resolver. No node change at all.

Telegram does not fit anything: HMAC verification requires the bot token, and
giving every operator a copy of it makes each one able to forge any user's
login. Recommendation: implement Telegram as an application-operated issuer
(P1.1) and document why it cannot be a first-class node route. That is a real
limitation, and a defensible one.

**Effort:** small for Farcaster (an adapter). Telegram: reuse P1.1.

---

### P1.5 Guest accounts

| | |
|---|---|
| **Privy** | A wallet before the user picks an identity, upgraded in place later. |
| **Signet today** | Every key ID is derived from a verified subject. A key with no subject cannot exist. |

The hard part is not creating the key — it is the *upgrade*. A guest key must
become bound to a real identity later without changing its address, or the user
loses whatever the guest wallet accumulated.

**Suggested path.** Two pieces, and the second is the one that matters:

1. An `anonymous` auth scheme where the client generates a keypair and the key
   ID is derived from its public key, gated by a group flag so an application
   opts in deliberately.
2. **Key subject rebinding** — an authenticated operation moving a key from one
   subject namespace to another, requiring proof of control of *both* the
   current binding and the new identity. Every node verifies both independently.

Rebinding is a genuinely new and genuinely dangerous primitive: it is a key
takeover by construction, and getting the authorization wrong means anyone can
steal a wallet. It wants its own design document and its own audit pass.
Recommendation: do not build it for guest accounts alone. Build it once, for
recovery (P2.2), and get guest upgrade for free — the mechanism is identical.

**Effort:** large. **Prerequisite for:** P2.2.

---

## Priority 2 — key lifecycle

### P2.1 Key export

| | |
|---|---|
| **Privy** | The user can export their private key. It is a headline non-custody claim. |
| **Signet today** | No mechanism. A key exists only as shares. |
| **Missing** | A threshold-authorized share reveal. |

This is the sharpest philosophical question in this document, so it is worth
being blunt about it: **export converts a threshold key into a single key.**
After export, the guarantee this entire network exists to provide is gone for
that key — permanently, and invisibly to anyone looking at the group.

It is also, genuinely, what non-custody means to a user, and refusing it
outright cedes ground to competitors who offer it.

**Suggested path.** `POST /v1/keys/export`, session-authenticated exactly as
signing is:

- The client generates an ephemeral X25519 keypair and includes the public key
  in the request.
- Each participating node verifies the session as it would for a signature,
  then returns *its share only*, encrypted to that ephemeral key.
- The client collects a threshold of shares and reconstructs locally. No node,
  and no coordinator, ever sees more than one share.
- The operation is recorded on every node as an audit event, and — worth
  considering — marks the key permanently in its metadata, so a group's key
  listing shows honestly which keys are still threshold-protected and which are
  not.

Additional controls worth having: a per-group `export_enabled` flag on the
contract, defaulting to **off**, so an application opts in; and a mandatory
timelock between request and reveal, so a compromised session cannot exfiltrate
a key before the user notices.

**Effort:** medium. **Node change:** a new endpoint and share-encryption path.
**Recommendation:** implement it, default it off, and make the console say
exactly what it costs.

---

### P2.2 Key recovery

| | |
|---|---|
| **Privy** | Password, cloud (iCloud/Drive), and user-passcode recovery. |
| **Signet today** | None. Lose access to the OAuth account and the key is unreachable. |
| **Missing** | A key resolvable by more than one credential. |

This is the largest *product* gap in this document. Today a user who loses their
Google account loses their wallet, with no path back — and unlike a centralized
provider, there is no support desk that can make an exception. That is the
correct security property and an unacceptable consumer experience, and the
tension is real rather than something better copy can resolve.

**Suggested path — multi-factor key binding.** Generalize a key's binding from
one subject to a *set* of accepted credentials with a policy:

```
key_id: oauth:https://accounts.google.com:1234
bindings:
  - { kind: oidc,     iss: accounts.google.com, sub: 1234 }
  - { kind: recovery, pubkey: 02…, added_at: … }   # a passkey, or a printed key
policy: { require: 1 }   # any one binding may open a session
```

Each node stores the binding set alongside the key and verifies any presented
credential against it independently. Adding a binding requires an existing one
plus a timelock, so an attacker who compromises a session cannot silently add
their own recovery factor and wait.

Social recovery — *k* of *n* guardians — is the same mechanism with
`require: k` over guardian public keys.

Note this subsumes P1.5's rebinding primitive: guest upgrade is adding an OIDC
binding to a key that has only an anonymous one, and then dropping the
anonymous binding.

**Effort:** large. Wants a design doc alongside `DESIGN-SCOPED-SUBKEYS.md`, and
an audit pass — the authorization rules here are the whole security of the
feature.

---

### P2.3 Key import

| | |
|---|---|
| **Privy** | Import an existing wallet. |
| **Signet today** | Absent. Already listed in `docs/PRODUCTION-GAPS.md` under "Critical". |

Without it there is no migration path off any competitor, which matters more
commercially than technically: every prospect with existing users has to be
told their wallets cannot come with them.

**Suggested path.** ECDSA first: the client performs a Feldman VSS split
locally and sends each node its share over an authenticated channel, with each
node verifying its share against the public commitments so a malicious client
cannot hand out inconsistent shares. FROST import needs compatible
`KeyPackage` construction — more work, and less urgent, since the EVM path is
where migrations will come from.

**Effort:** medium (ECDSA), large (FROST).

---

### P2.4 Solana as a production path

| | |
|---|---|
| **Privy** | Solana embedded wallets, first-class. |
| **Signet today** | `frost_ed25519` works end to end in tests and has no production consumer. `docs/CURVES.md` says so. |

Two concrete pieces are missing rather than the scheme itself:

- **Scope scheme `0x02`** is specified in `DESIGN-SCOPED-SUBKEYS.md` but the
  Solana transaction parser — extract the authority from the message, verify it
  against the scope — is not implemented. Without it there are no scoped Solana
  keys, which is precisely the agent use case.
- **Integration testing against real chain verification.** `PRODUCTION-GAPS.md`
  calls this out: nothing has verified a Signet Ed25519 signature through
  Solana's `Ed25519SigVerify` program.

**Effort:** medium. **Unblocks:** an entire chain ecosystem, from a scheme that
is already built and idle.

---

## Priority 3 — policy enforcement

The console's Policies screen labels every rule with where it is actually
enforced — `node`, `smart_account`, or `advisory` — and only key scopes are
`node` today. Everything below moves rules out of `advisory`.

### P3.1 Value and rate limits

| | |
|---|---|
| **Privy** | Server-side wallet policies: value caps, per-period limits, evaluated before signing. |
| **Signet today** | Scopes constrain *what* is signed, never *how much* or *how often*. |

`DESIGN-APP-COSIGNING.md` already frames the decision and reaches the right
conclusion: on-chain enforcement in the smart account is the destination,
enforced app co-signing is the bridge. That analysis is sound and this section
does not relitigate it.

**Recommendation: build the bridge, and keep saying it is a bridge.** A key can
require that every payload also carry a valid signature from a designated
approver key; participants refuse to contribute a share without it. The
application's policy gate becomes a *necessary input* to signing rather than a
trust assumption, and a leaked agent session can no longer sign alone.

Two details from that document are worth restating because they are the
difference between a real feature and a decorative one:

- **The co-signature must bind the request, not just the payload hash**, or an
  approval is replayable across requests.
- **Signet still does not understand the policy.** It refuses to sign anything
  the application did not bless. It cannot tell you what the daily limit is,
  and should not claim to.

The console's `policies` table already carries `enforced_by`; a co-signing rule
would be the first `node`-enforced entry beyond a scope.

**Effort:** medium. **Node change:** an approver-key field on a key, and a
verification step in the sign path.

---

### P3.2 Calldata-level scopes for plain transactions

Scope scheme `0x01` binds `(entryPoint, chainId, sender)` — the *account*, not
what it calls. A key scoped to a UserOperation can call anything that account
can reach. For agents that is usually too broad.

**Suggested path.** A scheme `0x04` binding `(chainId, target, selector)`, so a
key can be limited to `transfer(address,uint256)` on one token. The parsing is
simple and the verification is the same shape as `0x03`.

**Effort:** small. **Value:** high, given how much agent tooling signs plain
transactions rather than typed data.

---

### P3.3 Required signers

Already in `PRODUCTION-GAPS.md` under "Medium". An application cannot require
that a specific operator — its own node, in a hybrid deployment — participate
in every session.

This matters more than its priority suggests, because the hybrid configuration
is what the positioning sells to enterprise buyers: "run a majority of your own
operators". Without required signers, a hybrid group can produce signatures
that the customer's own nodes never saw, which is not the guarantee the sales
conversation implied.

**Suggested path.** A `required_signers` set on the group contract, checked by
every participant before it contributes.

**Effort:** small. **Recommendation:** raise its priority.

---

## Priority 4 — operations and management surface

### P4.1 User enumeration and deletion

The platform reconstructs an application's user list by parsing key IDs
(`backend/internal/api/handlers_keys.go`, `subjectHashFromKeyID`). It works, and
it is inference rather than an answer.

**Suggested path.** Two admin endpoints alongside `/admin/keys`:

- `POST /admin/users` — subjects with keys in a group, hashed, with counts and
  last-activity.
- `POST /admin/users/delete` — delete every key for a subject in one operation,
  which is what a GDPR erasure request actually requires. Doing it key by key
  from the console is not a compliance story anyone will accept.

**Effort:** small.

---

### P4.2 Node event stream

The platform's webhooks fire only for things it did itself. Anything the *nodes*
do — a key created by an application's backend, a session opened, a disable
executed — is invisible until the next manual sync.

**Suggested path.** An optional signed webhook from each node, sharing the
metering transport from P0.1. Signed per node, so a receiver can tell which
operator reported an event and notice when one stops reporting.

**Effort:** small once P0.1 exists.

---

### P4.3 Signed operator metadata

The marketplace directory is curated by platform staff
(`node_operators`, staff-gated). That makes this platform a trust point for
operator identity, which contradicts the rest of the design: everything else a
developer relies on is verifiable without us.

**Suggested path.** Operators publish a metadata document signed by their node
key, referenced by an on-chain pointer (an ENS text record, or a URI in the
factory's `NodeInfo`). The platform then *displays* metadata rather than
*asserting* it, and any client can verify the same thing independently. Staff
curation becomes a "reviewed" badge on top of self-published data — a claim
about diligence, not a claim about identity.

**Effort:** small-medium. **Value:** removes the platform from the trust path
for the marketplace.

---

### P4.4 Prometheus metrics and signed liveness

`/debug/stats` returns a JSON blob and nothing else. Two consequences: operators
cannot run normal monitoring, and the platform's uptime figures come from its
own probes — which measure "responds to the platform", not "serves users".

`docs/ECONOMIC-MODEL.md` §10 already anticipates signed liveness proofs for
metering. The same primitive fixes the marketplace's uptime numbers, and would
let an application verify an operator's liveness claim without trusting this
platform's probe.

**Effort:** small (metrics), medium (signed liveness).

---

### P4.5 The rest of `PRODUCTION-GAPS.md`

Not restated here — that document is the authority. The items that most affect
what this platform can honestly promise:

- **TLS on the node API.** Session keys transit in the clear today.
- **Rate limiting.** `/v1/auth` triggers proof verification; nothing bounds it.
- **Backup and recovery.** Node data loss below threshold is permanent key loss.
- **Signing retry with subset selection.** Without it, one griefing operator can
  block signing — which directly contradicts "no single operator can block
  signing", a claim the marketing makes and the console repeats.
- **Persistent audit logging.** stdout is not an audit trail.

---

## Explicitly out of scope for the node

Listed so they are not mistaken for gaps:

- **Funding and on-ramps.** They move value to an address; the address is all
  they need. Pure client integration. The console records the choice under
  Configuration → Funding and says exactly this.
- **Login-modal branding.** Application-side rendering. The platform stores the
  configuration; nothing reaches a node.
- **Team members, roles, invitations, billing UI.** Platform concerns.
- **Analytics dashboards.** Platform, once P0.1 provides the data.

---

## Suggested sequencing

| Order | Items | Why here |
|---|---|---|
| 1 | P0.1 metering, P0.2 circuits release | Nothing else in the platform is real without the first; the second is a release, not a project. |
| 2 | P3.3 required signers, P3.2 calldata scopes, P4.1 user admin | Small, high-value, and P3.3 makes the hybrid enterprise story true. |
| 3 | P1.2 passkeys, P1.4 Farcaster resolver | The two login methods that fit the existing architecture without new cryptography. |
| 4 | P2.4 Solana, P3.1 app co-signing | Unlock a built-and-idle scheme; ship the policy bridge the market is asking for. |
| 5 | P2.1 export, P2.3 import | Migration in and out. Commercially load-bearing. |
| 6 | P2.2 recovery (subsuming P1.5) | The largest design surface here. Worth doing once, properly, rather than twice. |
| 7 | P1.1 OTP issuer, P1.3 ES256 circuits | Broadens the addressable set; neither blocks anything. |

Throughout: `PRODUCTION-GAPS.md` critical items in parallel. None of the above
should ship to real users over a plaintext, unrate-limited API.
