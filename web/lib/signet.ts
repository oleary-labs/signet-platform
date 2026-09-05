"use client";

import { API_BASE } from "./api";
import type { NetworkConfig } from "./types";

/**
 * Client-side sign-in.
 *
 * Two routes, both of which end in the same place — a signature the platform
 * verifies server-side before it will open a session.
 *
 * 1. **Signet.** The console dogfoods the protocol: the developer authenticates
 *    with OAuth, proves possession of that credential to the bootstrap group in
 *    zero knowledge, and the group threshold-signs the platform's challenge.
 *    The platform verifies that FROST signature against the group's public key.
 *    Nothing replayable ever reaches the platform, and no single node could
 *    have produced the signature.
 *
 * 2. **Sign-In with Ethereum.** A plain ERC-4361 message signed by an EOA, for
 *    node operators and teams who have not onboarded through Signet yet.
 */

const NONCE_KEY = "signet_login_nonce";

export interface SignetLoginProgress {
  stage:
    | "challenge"
    | "oauth"
    | "session-key"
    | "proving"
    | "registering"
    | "keygen"
    | "signing"
    | "verifying";
  message: string;
}

/** Human-readable copy for each stage, so the sign-in screen can say what is
 *  actually happening rather than spinning silently through a ten-second proof. */
export const STAGE_COPY: Record<SignetLoginProgress["stage"], string> = {
  challenge: "Requesting a sign-in challenge",
  oauth: "Redirecting to your identity provider",
  "session-key": "Generating an ephemeral session key",
  proving: "Proving your credential in zero knowledge",
  registering: "Registering the session with the signing group",
  keygen: "Locating your key across the group",
  signing: "Asking the group to sign the challenge",
  verifying: "Verifying the threshold signature",
};

export function rememberNonce(nonce: string) {
  sessionStorage.setItem(NONCE_KEY, nonce);
}

export function recallNonce(): string | null {
  return sessionStorage.getItem(NONCE_KEY);
}

export function forgetNonce() {
  sessionStorage.removeItem(NONCE_KEY);
}

/* ────────────────────────── Sign-In with Ethereum ────────────────────────── */

interface Eip1193Provider {
  request(args: { method: string; params?: unknown[] }): Promise<unknown>;
}

export function injectedProvider(): Eip1193Provider | null {
  if (typeof window === "undefined") return null;
  const eth = (window as { ethereum?: Eip1193Provider }).ethereum;
  return eth ?? null;
}

/** Build the ERC-4361 message. The domain, chain, and nonce all sit inside the
 *  signed text, which is what stops a signature made elsewhere being replayed
 *  here — the server checks each one against what it issued. */
export function buildSiweMessage(opts: {
  domain: string;
  address: string;
  statement: string;
  uri: string;
  chainId: number;
  nonce: string;
}): string {
  return [
    `${opts.domain} wants you to sign in with your Ethereum account:`,
    opts.address,
    "",
    opts.statement,
    "",
    `URI: ${opts.uri}`,
    "Version: 1",
    `Chain ID: ${opts.chainId}`,
    `Nonce: ${opts.nonce}`,
    `Issued At: ${new Date().toISOString()}`,
  ].join("\n");
}

export async function siweSignIn(challenge: {
  nonce: string;
  statement: string;
  domain?: string;
  chain_id?: number;
  uri?: string;
}): Promise<{ address: string; message: string; signature: string }> {
  const provider = injectedProvider();
  if (!provider) {
    throw new Error(
      "No Ethereum wallet was found in this browser. Install one, or sign in with Signet instead.",
    );
  }

  const accounts = (await provider.request({ method: "eth_requestAccounts" })) as string[];
  const address = accounts?.[0];
  if (!address) throw new Error("Your wallet did not return an account.");

  const message = buildSiweMessage({
    domain: challenge.domain ?? window.location.host,
    address,
    statement: challenge.statement,
    uri: challenge.uri ?? window.location.origin,
    chainId: challenge.chain_id ?? 1,
    nonce: challenge.nonce,
  });

  const signature = (await provider.request({
    method: "personal_sign",
    params: [message, address],
  })) as string;

  return { address, message, signature };
}

/* ─────────────────────────── Signet (dogfood) ─────────────────────────── */

/**
 * Ask the bootstrap group to threshold-sign a digest.
 *
 * The request is signed with the ephemeral session key, and each node verifies
 * that signature independently before contributing its share, so the platform
 * relaying the call adds no authority to it.
 */
export async function signWithBootstrapGroup(opts: {
  network: NetworkConfig;
  sessionKeypair: { privateKey: Uint8Array; publicKeyHex: string };
  claims: { iss: string; sub: string; exp: number; aud: string; azp: string };
  digestHex: string;
  /** Set for auth-key certificate sessions. The nodes derive the key id from
   *  the identity rather than from the claims, so it has to be signed over
   *  here too or the request reads as a signature failure. */
  identity?: string;
}): Promise<string> {
  const { signSignRequest } = await import("@oleary-labs/signet-sdk/request");

  const digest = hexToBytes(opts.digestHex);
  const signed = await signSignRequest(
    opts.sessionKeypair as never,
    opts.claims as never,
    opts.network.bootstrap_group,
    digest,
    undefined,
    opts.identity,
  );

  const node = opts.network.bootstrap_nodes[0];
  if (!node) throw new Error("This deployment has no bootstrap nodes configured.");

  const res = await fetch(`${API_BASE}/v1/node/bootstrap-proxy`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "x-node-url": node,
      "x-node-path": "/v1/sign",
    },
    body: JSON.stringify({ ...signed, curve: "frost_secp256k1" }),
  });

  const text = await res.text();
  if (!res.ok) throw new Error(`The group could not sign the challenge: ${text}`);

  const body = JSON.parse(text) as {
    ethereum_signature?: string;
    signature?: string;
  };
  const signature = body.ethereum_signature ?? body.signature;
  if (!signature) throw new Error("The group returned no signature.");
  return signature;
}

/** Derive the counterfactual SignetAccount address for a group public key.
 *  Returned for display and for the group-attach check; the group key is what
 *  actually identifies the user. */
export async function counterfactualAccount(
  network: NetworkConfig,
  groupPublicKey: string,
): Promise<string | null> {
  if (!network.account_factory_address || network.account_factory_address === "0x") return null;
  try {
    const { createPublicClient, http } = await import("viem");
    const client = createPublicClient({ transport: http(network.rpc_url) });
    const address = await client.readContract({
      address: network.account_factory_address as `0x${string}`,
      abi: [
        {
          type: "function",
          name: "getAddress",
          stateMutability: "view",
          inputs: [
            { name: "entryPoint", type: "address" },
            { name: "groupPublicKey", type: "bytes" },
            { name: "salt", type: "uint256" },
          ],
          outputs: [{ name: "", type: "address" }],
        },
      ],
      functionName: "getAddress",
      args: [
        network.entrypoint_address as `0x${string}`,
        normalizeHex(groupPublicKey) as `0x${string}`,
        0n,
      ],
    });
    return address as string;
  } catch {
    // The account address is a convenience, not a credential. If the RPC
    // endpoint is unreachable, sign-in should still complete.
    return null;
  }
}

function normalizeHex(v: string): string {
  return v.startsWith("0x") ? v : `0x${v}`;
}

export function hexToBytes(hex: string): Uint8Array {
  const clean = hex.startsWith("0x") ? hex.slice(2) : hex;
  const out = new Uint8Array(clean.length / 2);
  for (let i = 0; i < out.length; i++) {
    out[i] = parseInt(clean.slice(i * 2, i * 2 + 2), 16);
  }
  return out;
}

export function bytesToHex(bytes: Uint8Array): string {
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}
