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
