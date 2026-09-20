"use client";

import { useRouter } from "next/navigation";
import { Suspense, useCallback, useEffect, useRef, useState } from "react";
import { Logo } from "@/components/Logo";
import { ErrorNote } from "@/components/ui";
import { api } from "@/lib/api";
import { useSession } from "@/providers/SessionProvider";
import {
  STAGE_COPY,
  counterfactualAccount,
  forgetNonce,
  recallNonce,
  signWithBootstrapGroup,
  type SignetLoginProgress,
} from "@/lib/signet";
import { API_BASE } from "@/lib/api";
import { storeSigningSession } from "@/lib/session-key";

export default function CallbackPage() {
  return (
    <Suspense fallback={null}>
      <Callback />
    </Suspense>
  );
}

/**
 * Completes the Signet sign-in after the OAuth redirect.
 *
 * The whole flow lives here rather than in a provider because it is a one-shot
 * sequence with a visible progress state — the proof step alone takes seconds,
 * and a developer watching a blank screen has no way to tell it apart from a
 * hang.
 */
function Callback() {
  const router = useRouter();
  const { network, onSignedIn } = useSession();
  const [stage, setStage] = useState<SignetLoginProgress["stage"]>("session-key");
  const [error, setError] = useState<unknown>(null);
  const started = useRef(false);

  const run = useCallback(async () => {
    if (!network) return;
    const nonce = recallNonce();
    if (!nonce) {
      throw new Error(
        "The sign-in challenge was lost. This usually means the tab was reloaded mid-flow — start again.",
      );
    }

    setStage("session-key");
    const { handleOAuthCallback, decodeIdToken } = await import("@oleary-labs/signet-sdk/oauth");
    const { generateSessionKeypair } = await import("@oleary-labs/signet-sdk/session");

    const jwt = await handleOAuthCallback("/api/auth/token");
    const claims = decodeIdToken(jwt);
    if (claims.exp * 1000 < Date.now()) {
      throw new Error("The identity token had already expired. Sign in again.");
    }
    const keypair = await generateSessionKeypair();

    setStage("proving");
    // The console proves server-side, through the bundler's /v1/prove.
    //
    // The stronger client-side path — where the credential never leaves the
    // device — is what an application built on the SDK should use, and is
    // configurable per app under Embedded wallets. It is not used *by the
    // console itself* because the SDK's browser proving path needs
    // `@oleary-labs/signet-circuits` at a version that is not published to npm
    // yet (the SDK peer-requires ^0.3.0; only 0.1.0 is available). Wiring it in
    // regardless would produce a console that fails to build. See
    // FEATURE_EXPANSION.md.
    const { generateServerProof } = await import("@oleary-labs/signet-sdk/server-prover");
    const { proof, jwksModulus: modulus } = await generateServerProof(
      "/api/bundler",
      jwt,
      keypair.publicKeyHex,
    );

    setStage("registering");
    const proxy = `${API_BASE}/v1/node/bootstrap-proxy`;

    // One node opens the session, and the same one serves the rest of sign-in.
    //
    // /v1/auth broadcasts: the node that receives it tells the others, and each
    // re-verifies independently rather than trusting the initiator. So asking
    // every node is redundant work, and authenticateWithBootstrap's shape makes
    // it worse than redundant — it fans out and returns on the first success,
    // reporting the rest to console.warn. A node that *rejected* the session
    // therefore looked the same as one that accepted it, and keygen and signing
    // would go on to pick bootstrap_nodes[0] regardless. When that was the node
    // that failed, sign-in died at the last step with "unauthorized" and a
    // perfectly good session on the other two.
    //
    // Pinning removes the propagation window as well: the node being asked to
    // sign is the one the session was created on, rather than one waiting for a
    // broadcast to arrive. This is how authkey-session, delegate and
    // resolver-session already work; bootstrap is the oldest module and the
    // only one that fanned out.
    const sessionNode = network.bootstrap_nodes[0];
    if (!sessionNode) throw new Error("This deployment has no bootstrap nodes configured.");

    const { authenticateWithBootstrap } = await import("@oleary-labs/signet-sdk/bootstrap");
    await authenticateWithBootstrap(
      {
        groupId: network.bootstrap_group,
        nodeUrls: [sessionNode],
        proxyEndpoint: proxy,
      },
      proof,
      keypair.publicKeyHex,
      claims,
      modulus,
    );

    setStage("keygen");
    const { keygen } = await import("@oleary-labs/signet-sdk/keygen");
    const key = await keygen(
      {
        groupId: network.bootstrap_group,
        // The session's node first. keygen treats this list as candidate
        // initiators rather than participants — the DKG runs across the whole
        // group whichever one starts it — so the others stay as transport
        // failover for a node that stops answering mid-sign-in.
        nodeUrls: [sessionNode, ...network.bootstrap_nodes.filter((n) => n !== sessionNode)],
        proxyEndpoint: proxy,
      },
      keypair,
      claims,
    );

    setStage("signing");
    // The digest is recomputed here from the same message the server built, so
    // the signature covers a challenge this client actually saw rather than an
    // opaque blob the server handed over.
    const challenge = await api.challenge("signet");
    const signature = await signWithBootstrapGroup({
      network,
      sessionKeypair: keypair,
      claims,
      digestHex: challenge.digest!,
      nodeUrl: sessionNode,
    });

    setStage("verifying");
    const account = await counterfactualAccount(network, key.groupPublicKey);
    const result = await api.verify({
      method: "signet",
      nonce: challenge.nonce,
      signature,
      group_public_key: key.groupPublicKey,
      account: account ?? undefined,
    });

    // Keep the ephemeral signing session for this tab so the console can ask
    // the group to sign on-chain actions without re-proving the credential on
    // every click. See lib/session-key.ts for what that does and does not
    // authorize.
    storeSigningSession({ keypair, claims, groupPublicKey: key.groupPublicKey, nodeUrl: sessionNode });

    forgetNonce();
    onSignedIn(result.session, result.token);

    // Record the smart wallet this key controls. Every on-chain action the
    // console takes is sent from it, and the platform checks operations
    // against it — so an account without one cannot do anything on-chain, and
    // this is the moment its address is first known.
    await api.recordSignetKey({
      group_public_key: key.groupPublicKey,
      key_id: key.keyId,
      key_address: key.ethereumAddress,
      smart_account: account ?? "",
    });

    const { getOAuthReturnTo } = await import("@oleary-labs/signet-sdk/oauth");
    router.replace(getOAuthReturnTo() || "/apps");
  }, [network, onSignedIn, router]);

  useEffect(() => {
    if (!network || started.current) return;
    started.current = true;
    run().catch(setError);
  }, [network, run]);

  return (
    <main className="flex min-h-screen items-center justify-center px-6">
      <div className="w-full max-w-md text-center">
        <Logo size={40} className="mx-auto text-fg" />

        {error ? (
          <div className="mt-8 text-left">
            <ErrorNote error={error} />
            <button
              type="button"
              onClick={() => router.replace("/login")}
              className="btn-ghost mt-4 w-full"
            >
              Back to sign in
            </button>
          </div>
        ) : (
          <>
            <h1 className="mt-8 text-[20px] font-semibold tracking-tight text-fg">
              {STAGE_COPY[stage]}
            </h1>
            <p className="mt-2.5 text-sm leading-relaxed text-muted">
              {stage === "proving"
                ? "Your credential is being proved without leaving this device. This is the slow step — a few seconds — and it happens once per sign-in."
                : "This takes a moment."}
            </p>

            <ol className="mx-auto mt-8 max-w-xs space-y-2.5 text-left">
              {(
                ["session-key", "proving", "registering", "keygen", "signing", "verifying"] as const
              ).map((s) => {
                const order = ["session-key", "proving", "registering", "keygen", "signing", "verifying"];
                const done = order.indexOf(s) < order.indexOf(stage);
                const active = s === stage;
                return (
                  <li key={s} className="flex items-center gap-3 text-[13.5px]">
                    <span
                      className={`flex h-5 w-5 flex-none items-center justify-center rounded-full border text-[10px] ${
                        done
                          ? "border-success-500 bg-success-500 text-white"
                          : active
                            ? "border-accent-500 text-accent-600"
                            : "border-fg/15 text-faint"
                      }`}
                    >
                      {done ? "✓" : ""}
                    </span>
                    <span className={active ? "text-fg" : done ? "text-muted" : "text-faint"}>
                      {STAGE_COPY[s]}
                    </span>
                  </li>
                );
              })}
            </ol>
          </>
        )}
      </div>
    </main>
  );
}
