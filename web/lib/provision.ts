"use client";

import { API_BASE, api } from "./api";
import { storeSigningSession } from "./session-key";
import { counterfactualAccount } from "./signet";
import type { NetworkConfig, User } from "./types";

/**
 * Giving every developer a Signet key, however they signed in.
 *
 * The console's own smart wallet is controlled by a Signet key, not by a
 * wallet. So signing in is only half of getting an account: the other half is
 * holding a key in the platform's signing group, and that has to happen the
 * same way for someone who used Google as for someone who used MetaMask.
 *
 * On the Signet route the sign-in *is* the key — proving an OAuth credential to
 * the group produces one. On the wallet route it is not: a SIWE signature
 * proves an address and nothing more. So the platform stands in as the identity
 * provider for its own console. It verifies the signature, then signs an
 * auth-key certificate binding the developer's identity to an ephemeral session
 * key, and the nodes accept that certificate because the group trusts the
 * platform's authorization key on-chain.
 *
 * The effect is the one worth having: a wallet user signs once, in MetaMask, to
 * prove who they are — and never again. Everything after that is signed by
 * their Signet key. The platform can issue a certificate but cannot use the key
 * it unlocks: the shares live with the operators, and only the holder of the
 * session key can ask them to sign.
 */

export type ProvisionStage =
  | "session"
  | "certificate"
  | "authenticating"
  | "keygen"
  | "wallet"
  | "recording"
  | "done";

export const PROVISION_STAGE_COPY: Record<ProvisionStage, string> = {
  session: "Creating a session key",
  certificate: "Requesting a certificate",
  authenticating: "Authenticating with your operators",
  keygen: "Generating your Signet key",
  wallet: "Deriving your smart wallet",
  recording: "Finishing up",
  done: "Ready",
};

export interface ProvisionResult {
  groupPublicKey: string;
  keyId: string;
  keyAddress: string;
  smartAccount: string | null;
}

/** Whether this user still needs a key, so callers can skip the work quietly
 *  rather than regenerating on every page load. */
export function needsSignetKey(user: User | null): boolean {
  return !!user && !user.signet_key_address;
}

/**
 * Provision a Signet key and its smart wallet for the signed-in developer.
 *
 * Safe to call again: keygen returns the existing key on a repeat, so a user
 * who clears their tab storage gets the same key and the same wallet back
 * rather than a second one.
 */
export async function provisionSignetKey(
  network: NetworkConfig,
  onStage?: (stage: ProvisionStage) => void,
): Promise<ProvisionResult> {
  if (!network.bootstrap_group || network.bootstrap_nodes.length === 0) {
    throw new Error(
      "This deployment has no bootstrap signing group configured, so no Signet key can be issued.",
    );
  }

  const [{ generateSessionKeypair }, { keygen }] = await Promise.all([
    import("@oleary-labs/signet-sdk/session"),
    import("@oleary-labs/signet-sdk/keygen"),
  ]);

  onStage?.("session");
  const keypair = await generateSessionKeypair();

  onStage?.("certificate");
  const cert = await api.nodeCertificate(keypair.publicKeyHex);

  onStage?.("authenticating");
  const authRes = await fetch(`${API_BASE}/v1/node/bootstrap-proxy`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "x-node-url": cert.node_urls[0],
      "x-node-path": "/v1/auth",
    },
    body: JSON.stringify({
      group_id: cert.group_id.toLowerCase(),
      session_pub: keypair.publicKeyHex,
      certificate: cert.certificate,
    }),
  });
  if (!authRes.ok) {
    const body = await authRes.text();
    // The most likely cause is worth naming: an operator that does not trust
    // the platform's authorization key rejects every certificate, and the
    // message alone ("untrusted authorization key") does not say whose.
    throw new Error(
      `Your operators would not accept the platform's certificate: ${body.trim()}. ` +
        "If this is a fresh deployment, the platform's authorization key may not be registered on the bootstrap group yet.",
    );
  }

  onStage?.("keygen");
  // The identity is the namespace the key lives in, and the nodes derive the
  // key id from it — so it must be the same string the certificate carried.
  const claims = { iss: "", sub: "", exp: cert.certificate.expiry, aud: "", azp: "" };
  const key = await keygen(
    {
      nodeUrls: cert.node_urls,
      groupId: cert.group_id,
      proxyEndpoint: `${API_BASE}/v1/node/bootstrap-proxy`,
    },
    keypair,
    claims as never,
    undefined,
    cert.identity,
    "frost_secp256k1",
  );
  if (!key.groupPublicKey) {
    throw new Error("Your operators generated a key but returned no public key for it.");
  }

  onStage?.("wallet");
  const smartAccount = await counterfactualAccount(network, key.groupPublicKey);

  // Keep the session so on-chain actions in this tab can be signed without
  // another round trip. It is tab-scoped and ephemeral by design.
  storeSigningSession({
    keypair,
    claims: claims as never,
    groupPublicKey: key.groupPublicKey,
    identity: cert.identity,
  });

  onStage?.("recording");
  await api.recordSignetKey({
    group_public_key: key.groupPublicKey,
    key_id: key.keyId,
    key_address: key.ethereumAddress,
    smart_account: smartAccount ?? "",
  });

  onStage?.("done");
  return {
    groupPublicKey: key.groupPublicKey,
    keyId: key.keyId,
    keyAddress: key.ethereumAddress,
    smartAccount,
  };
}

/**
 * Re-establish the signing session for this tab.
 *
 * The session key lives in sessionStorage, so it is gone when the tab closes —
 * deliberately. Getting it back is not the same for everyone:
 *
 * - A wallet user's key came from a platform-signed certificate, and the
 *   platform will sign another one for the session they already hold. No
 *   MetaMask prompt, which is the point: they signed once, at signup.
 * - A Signet user's key is bound to the OAuth credential they proved at login,
 *   so restoring it means proving that credential again.
 *
 * Keygen returns the existing key rather than making a second one, so this
 * yields the same key and therefore the same smart wallet.
 */
export async function restoreSigningSession(
  network: NetworkConfig,
  user: User,
  onStage?: (stage: ProvisionStage) => void,
): Promise<ProvisionResult> {
  if (user.subject_kind !== "siwe") {
    throw new Error(
      "Your Signet key is bound to the credential you signed in with, so restoring it means signing in again.",
    );
  }
  return provisionSignetKey(network, onStage);
}

/**
 * Whether this tab can get its signing key back without the developer signing
 * in again.
 *
 * This deliberately does not require an existing wallet. Someone whose key was
 * never issued — because provisioning failed at signup — is precisely who needs
 * this, and gating on the wallet they do not have told them to sign in again,
 * which would have failed the same way.
 */
export function canRestoreSigningSession(user: User | null): boolean {
  return !!user && user.subject_kind === "siwe";
}
