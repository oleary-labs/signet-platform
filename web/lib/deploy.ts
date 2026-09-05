"use client";

import { signetFactoryAbi } from "./abi";
import { signCall } from "./onchain";
import { USEROP_STAGE_COPY, type SignedUserOp, type UserOpStage } from "./userop";
import type { NetworkConfig, User } from "./types";

/**
 * Getting a signing group on-chain.
 *
 * There is one path, and the platform is not on it as a signer. The developer's
 * smart wallet calls `createGroup`, so it is the group's manager from the first
 * block — no interval where the platform holds the group, no handover that can
 * fail halfway, and no difference between how a group is created and how it is
 * managed afterwards.
 *
 * The wallet is controlled by their Signet key, which they hold however they
 * signed in. If it has not been deployed yet, the factory's initCode rides on
 * this same operation: the CREATE2 deployment and the group creation land
 * together, so a developer's first action needs no prior setup.
 *
 * This module signs and stops. The signed operation goes to the platform with
 * the request that needs it, and the platform decodes it before forwarding it
 * to the bundler. That is what makes it safe for us to pay: sponsorship is
 * granted by a route, not taken by a request.
 */

export type DeployStage = UserOpStage | "attaching";

export const DEPLOY_STAGE_COPY: Record<DeployStage, string> = {
  ...USEROP_STAGE_COPY,
  attaching: "Linking the group to your app",
};

export interface DeployRequest {
  network: NetworkConfig;
  user: User;
  nodes: string[];
  threshold: number;
  removalDelaySeconds: number;
  issuers: { issuer: string; clientIds: string[] }[];
  authKeys: string[];
  /** Development groups are sponsored; production groups pay their own gas. */
  sponsored?: boolean;
  onStage?: (stage: DeployStage) => void;
}

/**
 * Encode and sign `createGroup`, returning the operation for the platform to
 * submit.
 *
 * The issuers and authorization keys passed here are written by the same
 * transaction that creates the group, so a group is never briefly live with
 * nothing configured on it.
 */
export async function signCreateGroup(req: DeployRequest): Promise<SignedUserOp> {
  if (!req.network.factory_address) {
    throw new Error(
      "This deployment has no SignetFactory address configured, so no group can be created. Set FACTORY_ADDRESS on the API, or attach an existing group instead.",
    );
  }

  const { encodeFunctionData } = await import("viem");
  const callData = encodeFunctionData({
    abi: signetFactoryAbi,
    functionName: "createGroup",
    args: [
      req.nodes.map((n) => n as `0x${string}`),
      BigInt(req.threshold),
      BigInt(req.removalDelaySeconds),
      req.issuers.map((i) => ({ issuer: i.issuer, clientIds: i.clientIds })),
      req.authKeys.map((k) => (k.startsWith("0x") ? k : `0x${k}`) as `0x${string}`),
    ],
  });

  return signCall({
    network: req.network,
    user: req.user,
    dest: req.network.factory_address,
    callData,
    sponsored: req.sponsored,
    onStage: req.onStage,
  });
}
