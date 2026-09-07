# Signet Platform — context for Claude Code

@README.md

## What this repository is

The developer-facing console for the Signet protocol: a Next.js app and a Go API
over Postgres. `README.md` covers what it is and how to run it. This file
records the decisions that are easy to undo by accident.

## The rule that decides most design questions

**The platform must never become a trusted party.** If a change would let this
platform authorize a signature, alter a signing group, or become load-bearing at
runtime, it is the wrong change — no matter how much more convenient the console
becomes.

Three consequences show up constantly, and each has a comment at its site:

1. **No credential the nodes accept.** Reading a group's key inventory needs a
   signature from an authorization key the group trusts. The platform holds none
   and never will. `POST /v1/apps/{id}/keys/sync` therefore takes an inventory
   the *console* fetched with the developer's own key and caches it.
2. **No chain writes.** There is no server-side wallet. Everything that changes
   on-chain state is built in the browser and signed by the developer's own
   account — `web/lib/deploy.ts`, either a UserOperation through the bootstrap
   group or a plain transaction from their wallet.
3. **Verify before trusting a claim.** `POST /v1/apps/{id}/group/attach` reads
   the group contract and refuses the binding unless the caller's own account is
   its manager. A pasted address is not evidence.

## Say what is enforced where

The single most important thing this console communicates is the difference
between a rule **every operator enforces**, a rule **the chain enforces**, and a
note the developer's own server enforces. They must never look alike.

- `policies.enforced_by` is `node` | `smart_account` | `advisory`, set from the
  policy kind rather than by the user, and rendered as a badge on every row.
- Revoking a delegation records a revocation; it does not stop the token
  verifying. The response and the screen both say to disable the sub-key.
- Archiving an app does not stop its signing group. The response says so.
- Login methods the nodes cannot verify are shown under a warning, not beside
  the ones they can.

When adding a feature, work out which of the three it is *before* building the
UI. If it is `advisory`, say so on the screen.

## boot.sh

`./boot.sh` is the whole local stack: Postgres, anvil, contracts, three
`signetd` nodes, the API, seed data, and the console. Three things about it are
load-bearing:

- **The devnet is delegated, not reimplemented.** Chain, contracts, and nodes
  come from `../signet-protocol`'s own `devnet/start.sh`, so they cannot drift
  from the protocol. The script clones the repo if it is missing.
- **`force` vs `add`.** `force` is boot-owned wiring — contract addresses and
  inter-service URLs change every run, so a stale value in a `.env` must never
  win. `add` is user config, injected only where the file leaves the key unset.
  Neither ever writes to a component's `.env`.
- **Empty values in `.env.boot` are unset after sourcing.** Left exported, an
  empty `DATABASE_URL` shadows the same key in `backend/.env` — godotenv does
  not override a variable that is already present, even when it is blank.

The platform API listens on **8088**, not 8080: `signetd` binds 8080-8082 in a
local devnet.

## Backend

Conventions follow `../ASSEMBLY/oleary/backend`: chi router, pgxpool, embedded
SQL migrations applied at boot, `respond.JSON` / `respond.Fail`, all SQL in
`internal/store`.

**`internal/auth/frost.go`** — FROST threshold Schnorr verification. The
challenge is `expand_message_xmd(SHA-256)` per RFC 9380 with DST
`FROST-secp256k1-SHA256-v1chal`, reduced mod *n* **at full width**.
`ModNScalar.SetByteSlice` truncates past 32 bytes rather than reducing, which
silently produces a different challenge from the nodes and `FROSTVerifier.sol`.
Tested against the protocol's own vector — if that test fails, this code is
wrong, not the protocol.

**`internal/chain/abi.go`** — a purpose-built ABI codec. It supports exactly the
types the two Signet contracts use and errors on anything else rather than
guessing. Every accessor is bounds-checked; return data comes from an untrusted
RPC endpoint and must never panic a request.

**Composite literals in `if` headers** — Go cannot parse `if x, err := decoder{raw}.foo()`.
`GroupState` uses a `view` closure to avoid it.

**Caches carry `synced_at`.** Every table mirroring chain or node state is a
cache and says when it was read. Do not let a screen present one as current
without showing that.

**Audit writes never fail a request.** `store.Audit` logs and returns; a missing
log line is smaller than a rejected legitimate action.

## Web

Next.js App Router, Tailwind 3, no state library. `lib/hooks.ts` has a small
`useQuery` — the screens read one or two endpoints each, so a cache layer would
add more than it removes.

**Component classes live in `@layer components`** in `globals.css`. Without the
layer, `@apply w-full` inside `.select` beats a `w-auto` utility on the element
purely by source order, and width overrides silently stop working.

**Design tokens are Signet's own**, carried over from the earlier console (`archive/console-v1`): slate-blue
primary, sunset-orange accent, warm-stone neutrals. Accent is for the one
action that matters on a screen, plus selected and warning states. The motion
and surface vocabulary follows `../THASSA` and `../ASSEMBLY`.

**`/style-guide` imports the real components.** Do not reproduce a component
there — the point is that the page cannot drift from the product.

**Grey is not red.** Never-probed is not offline; not-synced is not broken.
`StatusDot` and the health rendering keep absence and failure distinct.

**The console proves server-side.** `@oleary-labs/signet-circuits` is not
published at a version the SDK's peer range accepts, so browser proving cannot
be installed from npm. See the comment in `app/auth/callback/page.tsx` and
P0.2 in `FEATURE_EXPANSION.md`. Do not wire the client path back in until that
release lands.

**The signing session is per-tab.** `lib/session-key.ts` keeps the ephemeral
session keypair in `sessionStorage` so the console can sign UserOperations
without re-proving on every click. It is an ephemeral key bound to a bounded
node-side session, not a root key, and it dies with the tab.

## Payments

Scaffolded and inert. `paymentsEnabled` is a constant `false`; nothing debits a
balance. Every billing response carries a note saying so. Do not remove those
notes without also implementing settlement.

## Node gaps

`FEATURE_EXPANSION.md` is the analysis of what the console can describe but the
node cannot yet enforce. When a gap closes, the caveat standing in for it in the
UI should go with it — they are written to be found together.
