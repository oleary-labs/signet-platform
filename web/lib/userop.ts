"use client";

import { signetAccountFactoryAbi } from "./abi";
import type { SigningSession } from "./session-key";
import { signWithBootstrapGroup } from "./signet";
import type { NetworkConfig } from "./types";

/**
 * Building and signing the operations the console sends on-chain.
 *
 * Every on-chain action a developer takes here — creating a group, adding a
 * login method, inviting an operator — is an ERC-4337 UserOperation from their
 * own smart wallet, signed by their Signet key. This module takes it as far as
 * a signature and stops.
 *
 * It deliberately does not submit. The signed operation goes back to whichever
 * console route asked for it, travels to the platform backend alongside that
 * request, and the backend forwards it to the bundler after decoding it and
 * checking it does what the route says (`backend/internal/userop`). That split
 * is what makes sponsorship safe to offer: the platform is paying, so the
 * platform gets to see what it is paying for.
 *
 * The signature never leaves the developer's control either way — the platform
 * can refuse to forward an operation, but it cannot alter one or produce one.
 */

/** The unpacked v0.7 wire form, matching what the backend expects and what
 *  signet-min-bundler's `FromRPC` reads. */
export interface SignedUserOp {
  sender: string;
  nonce: string;
  factory?: string;
  factoryData?: string;
  callData: string;
  callGasLimit: string;
  verificationGasLimit: string;
  preVerificationGas: string;
  maxFeePerGas: string;
  maxPriorityFeePerGas: string;
  paymaster?: string;
  paymasterData?: string;
  signature: string;
}

export type UserOpStage =
  | "building"
  | "sponsoring-stub"
  | "estimating"
  | "sponsoring"
  | "signing"
  | "submitting"
  | "confirming"
  | "done";

export const USEROP_STAGE_COPY: Record<UserOpStage, string> = {
  building: "Building the transaction",
  "sponsoring-stub": "Requesting gas sponsorship",
  estimating: "Estimating gas",
  sponsoring: "Finalising sponsorship",
  signing: "Signing with your Signet key",
  submitting: "Submitting",
  confirming: "Waiting for confirmation",
  done: "Done",
};

/** Where the console proxies bundler reads. Estimation and sponsorship have to
 *  happen before signing — they are inputs to the hash — so the browser talks
 *  to the bundler for those. Only the submission goes through the backend. */
const BUNDLER_PROXY = "/api/bundler";

export interface BuildUserOpRequest {
  network: NetworkConfig;
  session: SigningSession;
  /** The developer's smart wallet. Counterfactual until its first operation
   *  deploys it, which is why `factory`/`factoryData` may be present. */
  sender: string;
  dest: string;
  callData: string;
  /**
   * Ask the paymaster to pay.
   *
   * Only development actions are sponsored, and the platform refuses an
   * operation that carries paymaster data on a route that does not sponsor —
   * so this has to match what the app's environment allows, not merely what
   * the deployment is capable of.
   */
  sponsored?: boolean;
  onStage?: (stage: UserOpStage) => void;
}

export async function buildSignedUserOp(req: BuildUserOpRequest): Promise<SignedUserOp> {
  const { network, session } = req;

  if (!network.entrypoint_address) {
    throw new Error("This deployment has no EntryPoint address configured.");
  }
  if (!network.account_factory_address) {
    throw new Error("This deployment has no account factory address configured.");
  }

  const [{ buildUserOp, buildInitCode, fetchNonce, getUserOpHash }, bundler] = await Promise.all([
    import("@oleary-labs/signet-sdk/userop"),
    import("@oleary-labs/signet-sdk/bundler"),
  ]);

  const groupPublicKey = withPrefix(session.groupPublicKey) as `0x${string}`;

  req.onStage?.("building");
  const nonce = await fetchNonce(
    network.rpc_url,
    network.entrypoint_address as `0x${string}`,
    req.sender as `0x${string}`,
  );

  // Returns "0x" once the wallet is deployed, so the CREATE2 deployment rides
  // along with the developer's first action and never happens twice.
  const initCode = await buildInitCode(
    {
      rpcUrl: network.rpc_url,
      accountFactoryAddress: network.account_factory_address as `0x${string}`,
      accountFactoryAbi: signetAccountFactoryAbi as unknown as readonly Record<string, unknown>[],
      entryPointAddress: network.entrypoint_address as `0x${string}`,
    },
    req.sender as `0x${string}`,
    groupPublicKey,
  );

  let op = buildUserOp({
    sender: req.sender as `0x${string}`,
    nonce,
    initCode,
    dest: req.dest as `0x${string}`,
    callData: req.callData as `0x${string}`,
  });

  // Order is fixed and not rearrangeable: the stub makes the estimate account
  // for paymaster validation gas, and the real sponsorship signature commits to
  // the gas values, so it has to come after the estimate and before the hash.
  const usePaymaster =
    process.env.NEXT_PUBLIC_USE_PAYMASTER === "true" && (req.sponsored ?? true);

  if (usePaymaster) {
    req.onStage?.("sponsoring-stub");
    const stub = await bundler.getPaymasterStubData(
      BUNDLER_PROXY,
      network.entrypoint_address as `0x${string}`,
      network.chain_id,
      op,
    );
    op = bundler.applyPaymasterSponsorship(op, stub);
  }

  req.onStage?.("estimating");
  const gas = await bundler.estimateUserOpGas(
    BUNDLER_PROXY,
    network.entrypoint_address as `0x${string}`,
    op,
  );
  op.accountGasLimits = `0x${BigInt(gas.verificationGasLimit)
    .toString(16)
    .padStart(32, "0")}${BigInt(gas.callGasLimit).toString(16).padStart(32, "0")}` as `0x${string}`;
  op.preVerificationGas = BigInt(gas.preVerificationGas);

  if (usePaymaster) {
    req.onStage?.("sponsoring");
    const real = await bundler.getPaymasterData(
      BUNDLER_PROXY,
      network.entrypoint_address as `0x${string}`,
      network.chain_id,
      op,
    );
    op = bundler.applyPaymasterSponsorship(op, real);
  }

  req.onStage?.("signing");
  const opHash = getUserOpHash(
    op,
    network.entrypoint_address as `0x${string}`,
    network.chain_id,
  );
  op.signature = (await signWithBootstrapGroup({
    network,
    sessionKeypair: session.keypair as { privateKey: Uint8Array; publicKeyHex: string },
    claims: session.claims as never,
    digestHex: opHash,
    identity: session.identity,
  })) as `0x${string}`;

  return serialize(op);
}

/**
 * Flatten the packed operation into the unpacked v0.7 fields.
 *
 * The SDK keeps gas limits and fees as packed 32-byte pairs because that is
 * what the EntryPoint hashes; the RPC surface wants them apart. The paymaster
 * gas limits stay inside `paymasterData` — the bundler rebuilds
 * `paymasterAndData` by concatenating `paymaster` with it, so splitting them
 * out here would drop them from the blob the paymaster signed over.
 */
function serialize(op: {
  sender: string;
  nonce: bigint;
  initCode: string;
  callData: string;
  accountGasLimits: string;
  preVerificationGas: bigint;
  gasFees: string;
  paymasterAndData: string;
  signature: string;
}): SignedUserOp {
  const limits = op.accountGasLimits.slice(2).padStart(64, "0");
  const fees = op.gasFees.slice(2).padStart(64, "0");
  const init = op.initCode.slice(2);
  const pm = op.paymasterAndData.slice(2);

  return {
    sender: op.sender,
    nonce: `0x${op.nonce.toString(16)}`,
    factory: init.length >= 40 ? `0x${init.slice(0, 40)}` : "",
    factoryData: init.length > 40 ? `0x${init.slice(40)}` : "",
    callData: op.callData,
    callGasLimit: `0x${BigInt(`0x${limits.slice(32, 64)}`).toString(16)}`,
    verificationGasLimit: `0x${BigInt(`0x${limits.slice(0, 32)}`).toString(16)}`,
    preVerificationGas: `0x${op.preVerificationGas.toString(16)}`,
    maxFeePerGas: `0x${BigInt(`0x${fees.slice(32, 64)}`).toString(16)}`,
    maxPriorityFeePerGas: `0x${BigInt(`0x${fees.slice(0, 32)}`).toString(16)}`,
    paymaster: pm.length >= 40 ? `0x${pm.slice(0, 40)}` : "",
    paymasterData: pm.length > 40 ? `0x${pm.slice(40)}` : "",
    signature: op.signature,
  };
}

function withPrefix(v: string): string {
  return v.startsWith("0x") ? v : `0x${v}`;
}
