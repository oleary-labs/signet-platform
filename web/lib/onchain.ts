"use client";

import { useEffect, useState } from "react";

import { signetGroupAbi } from "./abi";
import { loadSigningSession } from "./session-key";
import { buildSignedUserOp, type SignedUserOp, type UserOpStage } from "./userop";
import type { NetworkConfig, User } from "./types";

/**
 * Signing the on-chain half of a console action.
 *
 * Several screens edit something that lives in two places at once: a login
 * method is a row in the platform's database *and* an issuer the group
 * contract trusts. The platform's copy is only a mirror — the operators read
 * the contract — so a change that touches one without the other leaves the
 * console showing something the nodes disagree with.
 *
 * These helpers produce the signed operation that goes to the backend with the
 * request, so both land together or neither does.
 */

export class NoSigningSessionError extends Error {
  constructor() {
    super(
      "The signing session for this tab has expired. Sign in again — the key that signs on-chain actions is deliberately not kept beyond the tab.",
    );
    this.name = "NoSigningSessionError";
  }
}

export interface SignCallRequest {
  network: NetworkConfig;
  user: User;
  /** The contract the call acts on. The backend checks this against the route,
   *  so it must be the app's own group (or the factory, for creation). */
  dest: string;
  callData: string;
  /** Whether this action is sponsored. Development is; production is not. */
  sponsored?: boolean;
  onStage?: (stage: UserOpStage) => void;
}

/** Build and sign one call from the developer's smart wallet. */
export async function signCall(req: SignCallRequest): Promise<SignedUserOp> {
  const session = loadSigningSession();
  if (!session) throw new NoSigningSessionError();

  if (!req.user.smart_account_address) {
    throw new Error(
      "Your account has no smart wallet yet. Sign out and back in so the console can provision your Signet key.",
    );
  }

  return buildSignedUserOp({
    network: req.network,
    session,
    sender: req.user.smart_account_address,
    dest: req.dest,
    callData: req.callData,
    sponsored: req.sponsored,
    onStage: req.onStage,
  });
}

/**
 * Sign a group-contract call, or return undefined when there is no group yet.
 *
 * An app can be configured before its group exists — issuers added then are
 * written into `createGroup` itself. Returning undefined lets the calling
 * screen use one code path for both, instead of branching on deployment state
 * at every call site.
 */
/** The group-contract methods the console can call. Naming them from the ABI
 *  keeps a typo from compiling into a call the backend would then refuse. */
export type GroupFunction = Extract<
  (typeof signetGroupAbi)[number],
  { type: "function" }
>["name"];

export async function signGroupCall(opts: {
  network: NetworkConfig;
  user: User;
  groupAddress: string | null | undefined;
  functionName: GroupFunction;
  args: readonly unknown[];
  sponsored?: boolean;
  onStage?: (stage: UserOpStage) => void;
}): Promise<SignedUserOp | undefined> {
  if (!opts.groupAddress) return undefined;

  const { encodeFunctionData } = await import("viem");
  const callData = encodeFunctionData({
    abi: signetGroupAbi,
    functionName: opts.functionName,
    args: opts.args as never,
  });

  return signCall({
    network: opts.network,
    user: opts.user,
    dest: opts.groupAddress,
    callData,
    sponsored: opts.sponsored,
    onStage: opts.onStage,
  });
}

/**
 * Whether this tab can sign on-chain actions right now.
 *
 * Reads sessionStorage, so it has to run after mount — computing it during
 * render would give one answer on the server and another in the browser, and
 * the button would flicker between enabled and disabled on hydration.
 */
export function useCanSignOnchain(user: User | null): boolean {
  const [ready, setReady] = useState(false);
  useEffect(() => {
    setReady(!!user?.smart_account_address && loadSigningSession() !== null);
  }, [user]);
  return ready;
}

/**
 * How a change reaches the chain.
 *
 * The two are not preference. Group management is `onlyManager`, so the call
 * has to come *from* the manager — which is the developer's smart wallet when
 * the console created the group, and their own EOA when it was created out of
 * band or handed over since. Sending the wrong one does not fall back; it
 * reverts, after the developer has watched a progress bar.
 */
export type Transport =
  | { kind: "userop"; from: string }
  | { kind: "wallet"; from: string }
  | { kind: "none"; reason: string };

/** What the API takes in place of a signature: one or the other, never both. */
export type OnchainProof = { user_op: SignedUserOp } | { transaction_hash: string };

/**
 * Decide which key signs, from the group's manager rather than from what this
 * tab happens to be able to do.
 *
 * Capability alone would get it wrong in the case that matters: a developer
 * signed in with Signet, holding a perfectly good session, acting on a group
 * whose manager is an EOA. They *can* build a user operation, and it would
 * revert.
 */
export function chooseTransport(
  user: User | null,
  manager: string | null | undefined,
  canSign: boolean,
): Transport {
  if (!user) return { kind: "none", reason: "You are not signed in." };
  if (!manager) return { kind: "none", reason: "This app has no signing group yet." };

  const smart = user.smart_account_address;
  if (smart && manager.toLowerCase() === smart.toLowerCase()) {
    return canSign
      ? { kind: "userop", from: smart }
      : {
          kind: "none",
          reason:
            "This group is managed by your Signet wallet, but this tab has no signing session. Sign in again.",
        };
  }

  const eoa = user.account_address;
  if (eoa && manager.toLowerCase() === eoa.toLowerCase()) {
    return { kind: "wallet", from: eoa };
  }

  return {
    kind: "none",
    reason: `This group is managed by ${manager}, which is not an account you signed in with. Sign in with that account to change it.`,
  };
}

/**
 * Send a group call as an ordinary transaction from the developer's wallet.
 *
 * Nothing is relayed and nothing is sponsored, so there is no operation to
 * build and no session to hold — the wallet signs, the chain executes, and the
 * platform is told afterwards. It waits for the receipt before returning
 * because the API confirms a mined transaction; handing it a pending hash
 * earns a 409 telling you to wait.
 */
export async function sendGroupCall(opts: {
  network: NetworkConfig;
  from: string;
  groupAddress: string;
  functionName: GroupFunction;
  args: readonly unknown[];
  onStage?: (stage: UserOpStage) => void;
}): Promise<{ transaction_hash: string }> {
  const provider = (globalThis as { ethereum?: EthereumProvider }).ethereum;
  if (!provider) {
    throw new Error(
      "No browser wallet is available, and this group is managed by an address only your wallet can act as.",
    );
  }

  const { encodeFunctionData } = await import("viem");
  const data = encodeFunctionData({
    abi: signetGroupAbi,
    functionName: opts.functionName,
    args: opts.args as never,
  });

  // Ask first: a wallet that is locked, or connected to a different account,
  // reports neither until something needs an account.
  const accounts = (await provider.request({ method: "eth_requestAccounts" })) as string[];
  const active = accounts?.[0];
  if (!active || active.toLowerCase() !== opts.from.toLowerCase()) {
    throw new Error(
      `Your wallet is on ${active ?? "no account"}, but this group is managed by ${opts.from}. Switch accounts and try again.`,
    );
  }

  opts.onStage?.("signing");
  const hash = (await provider.request({
    method: "eth_sendTransaction",
    params: [{ from: opts.from, to: opts.groupAddress, data }],
  })) as string;

  opts.onStage?.("submitting");
  await waitForReceipt(opts.network, hash);
  return { transaction_hash: hash };
}

interface EthereumProvider {
  request(args: { method: string; params?: unknown[] }): Promise<unknown>;
}

/**
 * Poll until the transaction is mined.
 *
 * Through the wallet's own provider rather than the configured RPC URL: the
 * public endpoint may be unset — it is, until an account factory exists — and
 * the wallet is already connected to the right chain by construction.
 */
async function waitForReceipt(network: NetworkConfig, hash: string): Promise<void> {
  const provider = (globalThis as { ethereum?: EthereumProvider }).ethereum;
  if (!provider) return;

  const deadline = Date.now() + 5 * 60_000;
  while (Date.now() < deadline) {
    const receipt = (await provider.request({
      method: "eth_getTransactionReceipt",
      params: [hash],
    })) as { status?: string } | null;
    if (receipt) {
      if (receipt.status && BigInt(receipt.status) === 0n) {
        throw new Error("The transaction reverted, so nothing changed on-chain.");
      }
      return;
    }
    await new Promise((r) => setTimeout(r, 3_000));
  }
  throw new Error(
    `The transaction did not confirm within five minutes. It may still land — check ${hash} before retrying.`,
  );
}

/**
 * Produce whatever proof the API needs for a group call, by whichever route
 * this group's manager requires.
 *
 * Call sites express the change — the method and its arguments — and spread
 * the result into the request. They should not be deciding transports: the
 * answer depends on who manages the group, which is a fact about the group and
 * not about the screen asking.
 */
export async function submitGroupCall(opts: {
  network: NetworkConfig;
  user: User;
  transport: Transport;
  groupAddress: string | null | undefined;
  functionName: GroupFunction;
  args: readonly unknown[];
  sponsored?: boolean;
  onStage?: (stage: UserOpStage) => void;
}): Promise<OnchainProof | undefined> {
  if (!opts.groupAddress) return undefined;
  if (opts.transport.kind === "none") throw new Error(opts.transport.reason);

  if (opts.transport.kind === "wallet") {
    return sendGroupCall({
      network: opts.network,
      from: opts.transport.from,
      groupAddress: opts.groupAddress,
      functionName: opts.functionName,
      args: opts.args,
      onStage: opts.onStage,
    });
  }

  const userOp = await signGroupCall({
    network: opts.network,
    user: opts.user,
    groupAddress: opts.groupAddress,
    functionName: opts.functionName,
    args: opts.args,
    sponsored: opts.sponsored,
    onStage: opts.onStage,
  });
  return userOp ? { user_op: userOp } : undefined;
}
