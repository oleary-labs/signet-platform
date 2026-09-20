"use client";

import type { IdTokenClaims, SessionKeypair } from "@oleary-labs/signet-sdk/types";

/**
 * The signing session held for the current browser tab.
 *
 * After the Signet sign-in route completes, the console keeps the ephemeral
 * session keypair so it can ask the bootstrap group to sign UserOperations —
 * creating a group, inviting an operator, rotating a key. Without it every
 * on-chain action would need a fresh OAuth round trip and ZK proof.
 *
 * It lives in sessionStorage, so it is scoped to one tab and gone when that tab
 * closes. What it authorizes is bounded: it is an *ephemeral* key that the
 * nodes accepted for a bounded session, not a root key, and it cannot outlive
 * the node-side session binding it was issued against. This mirrors what
 * signet-ui does, and the trade is deliberate — the alternative is re-proving
 * a credential for every click.
 */

const PRIV_KEY = "signet_session_priv";
const PUB_KEY = "signet_session_pub";
const CLAIMS_KEY = "signet_session_claims";
const GROUP_KEY = "signet_group_public_key";
const IDENTITY_KEY = "signet_session_identity";
const NODE_KEY = "signet_session_node";

export interface SigningSession {
  keypair: SessionKeypair;
  claims: IdTokenClaims;
  groupPublicKey: string;
  /**
   * Set when the session was established with an auth-key certificate rather
   * than an OAuth proof — which is every wallet sign-in.
   *
   * The nodes derive the key id from this identity instead of `iss:sub`, and
   * the client has to sign over the same one. Losing it does not fail loudly:
   * requests would be signed under the OAuth namespace and rejected as a bad
   * signature, so it is carried with the session rather than recomputed.
   */
  identity?: string;
  /**
   * The node this session was opened against.
   *
   * /v1/auth broadcasts, so every node learns the session — but the one that
   * received it holds it immediately and the others after a moment. Recording
   * it means later signing asks that node rather than whichever happens to be
   * first in the configured list, which is what made sign-in fail with
   * "unauthorized" while a perfectly good session existed elsewhere.
   */
  nodeUrl?: string;
}

export function storeSigningSession(s: SigningSession) {
  try {
    sessionStorage.setItem(PRIV_KEY, toHex(s.keypair.privateKey));
    sessionStorage.setItem(PUB_KEY, s.keypair.publicKeyHex);
    sessionStorage.setItem(CLAIMS_KEY, JSON.stringify(s.claims));
    sessionStorage.setItem(GROUP_KEY, s.groupPublicKey);
    if (s.identity) sessionStorage.setItem(IDENTITY_KEY, s.identity);
    if (s.nodeUrl) sessionStorage.setItem(NODE_KEY, s.nodeUrl);
    else sessionStorage.removeItem(IDENTITY_KEY);
  } catch {
    // Private-mode browsers throw. The console degrades to read-only for
    // on-chain actions and says so, rather than failing at click time.
  }
}

/**
 * How long before a session's real expiry we stop handing it out.
 *
 * A node session expires at the JWT's own `exp`, and group creation is several
 * round trips — a proof, gas estimation, paymaster data, threshold signing.
 * Handing out a session with thirty seconds left means discovering it is dead
 * partway through, which is the worst place to find out.
 */
const EXPIRY_SKEW_SECONDS = 120;

/** When this session stops being usable, or null if it carries no expiry. */
export function signingSessionExpiresAt(claims: IdTokenClaims): Date | null {
  return typeof claims.exp === "number" && claims.exp > 0 ? new Date(claims.exp * 1000) : null;
}

/**
 * The session for this tab, or null when there is none *or it has expired*.
 *
 * Expiry is checked here rather than at each call site because everything
 * downstream already treats null as "no session" and says something sensible —
 * useCanSignOnchain, chooseTransport, TransportNotice. Left unchecked, a dead
 * session looks alive right up until a node answers 401, and it does not even
 * answer accurately: expired entries are reaped in the background, so the node
 * reports "session not found" and the failure reads as though sign-in never
 * happened. It did; it was an hour ago.
 *
 * A node session lives exactly as long as the Google ID token it was proved
 * from — about an hour — and cannot be renewed from here. The JWT is used for
 * the proof and deliberately not kept, so there is nothing to re-prove with:
 * recovering means signing in again, not refreshing.
 */
export function loadSigningSession(): SigningSession | null {
  try {
    const priv = sessionStorage.getItem(PRIV_KEY);
    const pub = sessionStorage.getItem(PUB_KEY);
    const claims = sessionStorage.getItem(CLAIMS_KEY);
    const group = sessionStorage.getItem(GROUP_KEY);
    if (!priv || !pub || !claims || !group) return null;

    const parsed = JSON.parse(claims) as IdTokenClaims;
    const expiresAt = signingSessionExpiresAt(parsed);
    if (expiresAt && Date.now() >= expiresAt.getTime() - EXPIRY_SKEW_SECONDS * 1_000) {
      // Clear it rather than just refusing: keeping a dead session around means
      // every later read pays the same check and something eventually uses one
      // that slipped past it.
      clearSigningSession();
      return null;
    }
    return {
      keypair: { privateKey: fromHex(priv), publicKeyHex: pub },
      claims: JSON.parse(claims) as IdTokenClaims,
      groupPublicKey: group,
      identity: sessionStorage.getItem(IDENTITY_KEY) ?? undefined,
      nodeUrl: sessionStorage.getItem(NODE_KEY) ?? undefined,
    };
  } catch {
    return null;
  }
}

export function clearSigningSession() {
  try {
    [PRIV_KEY, PUB_KEY, CLAIMS_KEY, GROUP_KEY, IDENTITY_KEY, NODE_KEY].forEach((k) =>
      sessionStorage.removeItem(k),
    );
  } catch {
    /* ignore */
  }
}

function toHex(bytes: Uint8Array): string {
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

function fromHex(hex: string): Uint8Array {
  const clean = hex.startsWith("0x") ? hex.slice(2) : hex;
  const out = new Uint8Array(clean.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(clean.slice(i * 2, i * 2 + 2), 16);
  return out;
}
