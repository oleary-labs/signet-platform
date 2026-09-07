# Signet Platform

The developer-facing console for [Signet](https://github.com/oleary-labs/signet-protocol) —
where an application developer picks the operators who will hold shares of their
users' keys, sets a threshold, configures how their users sign in, and watches
what the network does for them.

A Next.js console and marketing site, backed by a small Go API over Postgres.

```
platform/
├── backend/   Go · chi · pgxpool — metadata, assets, metering, billing
├── web/       Next.js · TypeScript · Tailwind — console, marketplace, docs
└── FEATURE_EXPANSION.md   node-level gaps found while building this
```

---

## What this is, and what it deliberately is not

The platform stores **metadata and assets**. Authoritative state lives
elsewhere, and the split is not incidental — it is the product's central claim
made structural:

| State | Owner | This platform |
|---|---|---|
| Group membership, threshold, issuers, auth keys | `SignetGroup` on-chain | Reads and caches |
| Key material | Node KMS, in shares | Never sees it |
| Who may authorize a signature | The operator set | Holds no such credential |
| App config, branding, analytics, billing | This platform | Owns it |

Three properties follow, and the code is arranged so they stay true:

- **It cannot sign.** It holds no key any operator would accept. Fetching your
  own key inventory needs an authorization key the group trusts, which is why
  the console asks your browser to make that call and post the result back.
- **It never writes to a chain.** Every state-changing action in the console is
  a transaction or UserOperation built and signed in your browser, from your own
  account. There is no server-side wallet.
- **It is not in the path.** If this platform went away, every signing group
  would keep serving its users. Nothing here is load-bearing at runtime.

Where the platform's record and the network's behaviour can diverge — a revoked
delegation whose token still verifies, an archived app whose group keeps
running — the console says so on the screen rather than implying otherwise.

---

## Running it

```bash
./boot.sh
```

That is the whole thing. It brings up Postgres, anvil, the Signet contracts,
three `signetd` nodes with a 2-of-3 bootstrap group, the bundler if it is
present, the platform API on `:8088`, seed data, and the console on `:3000` —
then tails the logs. Ctrl-C stops everything it started.

The chain, contracts, and nodes come from `../signet-protocol` via its own
`devnet/start.sh`; the script clones the repo if it is missing rather than
reimplementing a devnet that would drift from the protocol's.

```bash
./boot.sh --no-kms      # nodes use in-process Go TSS (no Rust toolchain)
./boot.sh --auth        # seed Google as a trusted issuer on the bootstrap group
./boot.sh --no-nodes    # anvil + contracts only — fast, no signetd build
./boot.sh --no-chain    # platform against Postgres alone
./boot.sh --fresh       # drop and recreate the database first
./boot.sh --help        # every flag
```

Configuration lives in `.env.boot`, created from `.env.boot.example` on first
run. Everything has a working default; set `GOOGLE_CLIENT_ID` and
`GOOGLE_CLIENT_SECRET` to enable the dogfooded Signet sign-in route, otherwise
sign in with Ethereum.

Requires Go 1.25+, Node 20+, Foundry, `jq`, and either Docker or a local
Postgres. The nodes' production KMS additionally needs a Rust toolchain and
`protoc` (`brew install protobuf`); without them `boot.sh` falls back to the
protocol's in-process Go signing path and says so.

### Running the halves on their own

```bash
cd backend
cp .env.example .env          # set DATABASE_URL and SESSION_SECRET
createdb signet_platform
go run ./cmd/server           # migrations apply on boot
go run ./cmd/seed             # operator directory
```

```bash
cd web
npm install
cp .env.example .env.local    # point NEXT_PUBLIC_API_URL at the backend
npm run dev
```

Or bring up Postgres, the API, and the console in containers:

```bash
docker compose up
```

---

## Signing in

Two routes, and they are separate identities on purpose — proving control of an
EOA is not proof of control of a Signet account, so the console never merges
them.

**Signet** is the dogfooded route, and the one an application's own users take.
OAuth, then a zero-knowledge proof of that credential to the bootstrap group,
then the group threshold-signs a server-issued challenge. The API verifies that
FROST signature itself (`backend/internal/auth/frost.go`, checked against the
protocol's own test vector) before it will open a session. Nothing replayable
reaches the platform, and no single node could have produced the signature.

**Sign-In with Ethereum** is a plain ERC-4361 message from an EOA, for node
operators and teams who have not onboarded through Signet yet. It needs
`SIWE_DOMAIN` set; leaving it empty disables the route rather than accepting a
message for any domain.

---

## Backend layout

```
cmd/server/       entrypoint, background sync and probe jobs
cmd/seed/         development data
migrations/       embedded SQL, applied on boot
internal/
  api/            router, middleware, handlers
  auth/           FROST + SIWE verification, sessions, org authorization
  chain/          read-only eth_call against SignetFactory / SignetGroup
  nodeapi/        signetd client and the CORS proxy
  store/          every SQL statement in the project
  storage/        S3 (production) or local filesystem (development) uploads
  webhook/        signed event delivery
```

Two pieces are worth knowing about before changing them:

**`internal/auth/frost.go`** verifies FROST threshold Schnorr signatures
server-side. The challenge is `expand_message_xmd(SHA-256)` per RFC 9380,
reduced mod *n* at full width — truncating it would silently produce a different
challenge from the one the nodes and `FROSTVerifier.sol` compute. It is tested
against the protocol's own vector and the RFC's expansion vectors.

**`internal/chain/abi.go`** is a purpose-built ABI codec, not a general one. The
platform only ever performs `eth_call` against two known contracts, so it
supports exactly the types those functions use and refuses anything else rather
than guessing. Every accessor is bounds-checked — return data comes from an
untrusted RPC endpoint.

---

## Web layout

```
app/(marketing)/   landing, operator marketplace, status, pricing, docs, style guide
app/(console)/     the authenticated console
app/login          both sign-in routes
app/auth/callback  completes the Signet route
lib/deploy.ts      on-chain actions — UserOperation or wallet, never the server
lib/api.ts         typed client for the platform API
components/ui.tsx  the shared component vocabulary
```

The console is shaped the way a developer arriving from an embedded-wallet
product expects — users, wallets, analytics, then configuration — with one
section that has no equivalent there: **the signing group**. That is where the
operator set and threshold live, and it sits above settings because it is the
reason to be here.

Design tokens are Signet's own, carried over from the earlier console: slate-blue
primary, sunset-orange accent, warm-stone neutrals. The surface and motion
vocabulary follows the house style in `../THASSA` and `../ASSEMBLY` — an
IntersectionObserver reveal system, scroll-snapped landing sections, no
animation library. `/style-guide` renders the whole system from the same
components the product uses, so the two cannot drift.

No canonical Signet logo existed in any of the repositories, so the mark here is
drawn from the protocol's own idea: a seal whose ring is six separate arcs, one
picked out in the accent. The ring is visibly not one piece.

---

## Tests

```bash
cd backend && go test ./...
cd web && npm run typecheck && npm run build
```

The backend tests cover the parts where being wrong is silent: FROST
verification against the protocol's vector, SIWE parsing and replay rejection,
ABI decoding against hand-built fixtures and known ERC-20 selectors, scope
decoding, and webhook signature binding.

---

## Payments

Scaffolded and inert. The schema, the ledger, and invoice drafting are in place;
usage is metered and the console shows what it would cost at the published
per-active-wallet rate. **No code path debits a balance**, `payments_enabled` is
a constant `false`, and every billing response says so rather than letting the
screen imply money is moving.

---

## Related

| Repository | What it is |
|---|---|
| [signet-protocol](https://github.com/oleary-labs/signet-protocol) | The node, the KMS, and the group contracts |
| [signet-sdk](https://github.com/oleary-labs/signet-sdk) | The TypeScript client this console uses |
| [signet-circuits](https://github.com/oleary-labs/signet-circuits) | The Noir circuit proving an OAuth credential |
| [signet-wallet](https://github.com/oleary-labs/signet-wallet) | Smart account and on-chain FROST verifier |
| [signet-min-bundler](https://github.com/oleary-labs/signet-min-bundler) | Minimal ERC-4337 bundler and server-side prover |
| [`archive/console-v1`](https://github.com/oleary-labs/signet-platform/tree/archive/console-v1) | The earlier console this platform supersedes, kept on a branch of this repository |

[`FEATURE_EXPANSION.md`](FEATURE_EXPANSION.md) lists the node-level gaps found
while building this — what the console can describe but the network cannot yet
enforce, and a suggested path for each.
