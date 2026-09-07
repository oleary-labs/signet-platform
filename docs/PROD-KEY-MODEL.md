# Production Key Model — Splitting Manager, Admin, and User Roles

> **Carried forward.** Written against the previous Console (the `src/` tree
> this platform replaced), so its references to `signet-ui/CLAUDE.md` and
> `signet-ui/docs/TODO.md` point at files that now exist only on the
> `archive/console-v1` branch. The argument it makes is unchanged, and the
> console's manager / admin / consumer split implements it — the paths are the
> only stale part.

> **Status:** Design note, forward-looking. Not blocking the current Testnet 1
> cutover. The Console as it stands is demo-grade and intentionally optimized for
> "Google login → working group in 60 seconds." This doc describes what needs to
> change before the same code is suitable for production.

## The problem the current UI conflates three roles into one

The Console's group-creation wizard, by design, lets a user log in with Google
OAuth and immediately create a SignetGroup. The convenience is real, but it
collapses three architecturally distinct roles into a single OAuth identity:

1. **Group manager** — the Solidity `manager` field on `SignetGroup`. Holds
   `onlyManager` powers: `addAuthKey`, `removeAuthKey`, `queueAddIssuer`,
   `queueRemoval`, `transferManager`, etc. This is the governance role.
2. **Group admin authenticator** — the public key registered in `_authKeys` that
   gets used (via the auth-key certificate flow in `signet-sdk/admin.ts` and
   `node/auth.go`) to mint admin sessions on the protocol. This is the
   programmatic-admin role.
3. **Group user / API consumer** — an OAuth identity that's done keygen against
   the group and signs application payloads with their derived threshold keys.
   This is the everyday-usage role.

Today, all three end up tied to the Google account that ran the wizard:

- The wizard's `createGroup` UserOp is signed by the user's bootstrap-group
  threshold key, executing through the user's SignetAccount, so the SignetAccount
  ends up as `manager`.
- The wizard passes `[scheme_prefix(0x01) || user_threshold_pubkey]` as the only
  entry in `initialAuthKeys`, so the user's bootstrap threshold key is also the
  group's sole admin authenticator.
- The same OAuth identity that did all the above continues using the group as a
  regular consumer.

For a demo where the whole point is friction-free onboarding, this is a feature.
For production it's a serious failure mode: **loss of Google account =
loss of group**, with no recovery path. The same OAuth identity also can't be
revoked from one role without losing the others, multi-user groups are awkward,
and there is no offline / cold-storage path for governance actions.

## What production needs — the three roles split cleanly

| Role | Identity | Signing surface | Frequency |
|---|---|---|---|
| Manager | Hardware-wallet EOA (Ledger, MetaMask Snap, multisig) | Direct Sepolia/L1 transactions calling `onlyManager` functions | Rare — rotations, evictions, recovery |
| Admin authenticator | One or more entries in `_authKeys`; can be ECDSA (`0x00`, hardware-wallet or backend ECDSA key) or Schnorr (`0x01`, another Signet group's threshold key) | Signs `AuthCertificate` payloads consumed by `POST /v1/auth` | Occasional — admin API calls, automated ops |
| User / consumer | OAuth identity → ZK proof → bootstrap-group threshold key | Threshold-signs application payloads via `/v1/sign` | Constant — every user action |

Notably: the **contracts already model this correctly**. `SignetGroup.manager` is
just an `address`. `SignetGroup._authKeys` is an unconstrained list. The
protocol's `verifyAuthKeySignature` already dispatches on the scheme prefix
(`AuthKeySchemeECDSA = 0x00`, `AuthKeySchemeSchnorr = 0x01`). The model the
contracts express is the production model. The conflation is purely a UI
implementation choice — a single OAuth login backing three actions that should
have been three separate connection contexts.

## What the UI would need to change

The Console's `signet-ui/CLAUDE.md` makes a firm architectural claim:

> This is NOT a standard wagmi "connect wallet" app. Users authenticate via
> social login (OAuth)…

That claim is correct for the **user/consumer role**, and should stay that way
— Signet's whole pitch is that end-users authenticate with the credentials they
already have, not a wallet. But it shouldn't apply uniformly to the
**manager role**. A wallet-connect step makes sense for governance precisely
because governance is rare, high-stakes, and almost always done by someone who
already understands what a private key is.

Concretely, the production-ready wizard flow looks like:

1. **Manager connects a hardware wallet** (wagmi + viem connector, just for this
   step — not woven into the Console's identity model). They sign a direct
   `factory.createGroup(...)` transaction on-chain as themselves. No UserOp, no
   bootstrap group involvement, no paymaster. They pay the gas.
2. **`initialAuthKeys` is populated explicitly**, not derived from whoever is
   logged in:
   - At minimum, the manager's own ECDSA pubkey (`0x00 ||
     compressed_pubkey`) so they can authenticate admin API calls directly from
     their hardware wallet.
   - Optionally, additional user-threshold pubkeys (`0x01 || compressed_pubkey`)
     for collaborators who should also have admin powers via the
     cert-sign-through-protocol path.
3. **Users join separately** via the existing OAuth flow. They do keygen
   against the bootstrap group like today. The manager later runs
   `addAuthKey(theirThresholdPubkey)` to grant them admin power if appropriate;
   otherwise they stay as plain consumers.

Two distinct connection contexts in one app: wagmi-style for the manager
sidebar/governance pane, social-login for everything else. They don't need to
share session state or identity — they're answering different questions.

## What falls out of the split

Recovery becomes possible. Losing your OAuth account no longer means losing the
group — the manager (hardware wallet, offline backup) rotates your auth key and
you're back. Conversely, losing the manager hardware is a serious problem, but
it's a *bounded* problem you can prepare for (seed phrase, multisig manager,
co-signers) rather than a silent failure mode hiding inside an OAuth provider's
account-recovery policy.

Multi-user groups stop being awkward. Today the user who happens to run the
wizard "owns" the group and everyone else is second-class. With explicit
`_authKeys` population, adding a teammate is just one `addAuthKey` call by the
manager. Each user keeps their own social login, the group recognizes them
through their threshold-key cert, neither party can lock the other out.

Audit clarity improves. Today admin actions on a group are signed by "the user's
SignetAccount via UserOp, validated by the user's threshold key" — three layers
of indirection between the cause and the on-chain effect. In the split model
the manager's hardware-wallet signature is the cause, full stop. Easier to
review, easier to revoke, easier to multisig.

The bootstrap group becomes legibly special. Right now it's just another
SignetGroup, indistinguishable in the contract layer from any user-created
group. In a production model where most groups have hardware-wallet managers,
the bootstrap group is the one explicit exception — it's Signet-managed, has
no end-user manager, and its only job is to mint user threshold keys. That
specialness deserves to be visible somewhere obvious in the codebase (a typed
distinction, or at minimum a documented constant) rather than implicit in env
configuration.

## Intermediate step — show-once application key

The full hardware-wallet manager flow is a real engineering lift (wagmi
connector wiring, a separate signing context, EOA-funded create flow, hardware
device UX testing). The `signet-ui/docs/TODO.md` item "Standalone application
key generation — Generate a separate secp256k1 keypair during deploy with a
'save your key' show-once UX" is a useful intermediate.

In that intermediate model:

- The Console generates a fresh ECDSA keypair client-side at wizard time.
- The pubkey goes into `initialAuthKeys` (`0x00`-prefixed) — replacing the
  current behavior of registering the user's threshold key.
- The privkey is shown to the developer once and never persisted by the
  Console. They save it themselves.
- The user's OAuth identity remains the consumer role only — keygen, sign,
  use the group through the UI. Not admin.
- Admin actions (now or later) are done by signing AuthCertificates with the
  saved ECDSA key, exactly the same flow a hardware-wallet manager would use.

This unlocks recovery (you can save the key in 1Password or whatever) and
ends the conflation, without yet needing to wire up wagmi/hardware-wallet
connectors. The hardware-wallet path becomes a strict superset: "instead of
generating an ECDSA key in the browser, import a pubkey from your hardware
device."

## Implications for the protocol and SDK

Mostly already supported. Specifically:

- `SignetGroup`'s `_authKeys` already accepts arbitrary `0x00`-prefixed ECDSA
  entries. No contract change.
- `signet-sdk/src/authkey-session.ts` already implements ECDSA cert signing
  against the protocol — that's how a backend service authenticates today. The
  manager would use the same module, just with a different key source.
- The protocol's `AuthCertificate` verification (`node/auth.go`
  `verifyAuthKeySignature`) is already curve-agnostic — switch statement on the
  scheme prefix.

The work is in `signet-ui`:

- A wallet-connect surface for the manager role, isolated from the existing
  social-login provider tree.
- A "create-via-EOA" wizard branch that calls `factory.createGroup` as a normal
  on-chain transaction rather than via UserOp.
- UI for `addAuthKey` / `removeAuthKey` / `transferManager` as governance
  actions distinct from the consumer dashboard.
- Updated docs explicitly distinguishing the three roles, including which
  parts of the existing CLAUDE.md "Auth model — NOT a normal wallet dApp"
  guidance still applies (the consumer role) versus where it doesn't (the
  manager role).

## Related docs

- `signet-ui/CLAUDE.md` — current auth-model architecture decisions
- `signet-ui/docs/TODO.md` — "Standalone application key generation" tracker item
- `signet-protocol/docs/CURVES.md` — canonical reference for `frost_secp256k1` /
  `frost_ed25519` / `ecdsa_secp256k1` strings used in all signing operations
- `signet-protocol/node/auth.go` — `GroupAuth`, `IsAuthKeyTrusted`,
  `verifyAuthKeySignature` implementations
- `signet-protocol/contracts/contracts/SignetGroup.sol` — `_authKeys`,
  `addAuthKey`, `removeAuthKey`, `manager`, `onlyManager`
- `signet-sdk/src/authkey-session.ts` — ECDSA auth-key cert flow on the client
  side
