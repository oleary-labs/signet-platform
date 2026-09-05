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
}

export function storeSigningSession(s: SigningSession) {
  try {
    sessionStorage.setItem(PRIV_KEY, toHex(s.keypair.privateKey));
    sessionStorage.setItem(PUB_KEY, s.keypair.publicKeyHex);
    sessionStorage.setItem(CLAIMS_KEY, JSON.stringify(s.claims));
    sessionStorage.setItem(GROUP_KEY, s.groupPublicKey);
    if (s.identity) sessionStorage.setItem(IDENTITY_KEY, s.identity);
    else sessionStorage.removeItem(IDENTITY_KEY);
  } catch {
    // Private-mode browsers throw. The console degrades to read-only for
    // on-chain actions and says so, rather than failing at click time.
  }
}

export function loadSigningSession(): SigningSession | null {
  try {
    const priv = sessionStorage.getItem(PRIV_KEY);
    const pub = sessionStorage.getItem(PUB_KEY);
    const claims = sessionStorage.getItem(CLAIMS_KEY);
    const group = sessionStorage.getItem(GROUP_KEY);
    if (!priv || !pub || !claims || !group) return null;
    return {
      keypair: { privateKey: fromHex(priv), publicKeyHex: pub },
      claims: JSON.parse(claims) as IdTokenClaims,
      groupPublicKey: group,
      identity: sessionStorage.getItem(IDENTITY_KEY) ?? undefined,
    };
  } catch {
    return null;
  }
}

export function clearSigningSession() {
  try {
    [PRIV_KEY, PUB_KEY, CLAIMS_KEY, GROUP_KEY, IDENTITY_KEY].forEach((k) =>
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
