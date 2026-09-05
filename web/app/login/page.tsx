"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useCallback, useEffect, useState } from "react";
import { Logo } from "@/components/Logo";
import { Callout, ErrorNote } from "@/components/ui";
import { api } from "@/lib/api";
import { useSession } from "@/providers/SessionProvider";
import { injectedProvider, rememberNonce, siweSignIn } from "@/lib/signet";
import { PROVISION_STAGE_COPY, provisionSignetKey } from "@/lib/provision";

export default function LoginPage() {
  return (
    <Suspense fallback={null}>
      <LoginScreen />
    </Suspense>
  );
}

function LoginScreen() {
  const router = useRouter();
  const params = useSearchParams();
  const { status, network, onSignedIn } = useSession();
  const [busy, setBusy] = useState<"signet" | "siwe" | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [provisioning, setProvisioning] = useState<string | null>(null);

  const next = params.get("next") || "/apps";

  useEffect(() => {
    if (status === "signed-in") router.replace(next);
  }, [status, router, next]);

  const signInWithSignet = useCallback(async () => {
    setBusy("signet");
    setError(null);
    try {
      const challenge = await api.challenge("signet");
      rememberNonce(challenge.nonce);

      const clientId = process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID;
      if (!clientId) {
        throw new Error(
          "Google sign-in is not configured on this deployment (NEXT_PUBLIC_GOOGLE_CLIENT_ID is unset). " +
            "Sign in with a wallet instead, or set it and restart.",
        );
      }
      if (!network?.bootstrap_group || network.bootstrap_nodes.length === 0) {
        throw new Error(
          "This deployment has no bootstrap signing group configured, so the Signet sign-in route " +
            "cannot run. Set BOOTSTRAP_GROUP and BOOTSTRAP_NODES on the API, or sign in with a wallet.",
        );
      }

      const { startGoogleOAuth } = await import("@oleary-labs/signet-sdk/oauth");
      await startGoogleOAuth({ clientId, callbackPath: "/auth/callback" }, next);
    } catch (err) {
      setError(err);
      setBusy(null);
    }
  }, [network, next]);

  const signInWithWallet = useCallback(async () => {
    setBusy("siwe");
    setError(null);
    setProvisioning(null);
    try {
      const challenge = await api.challenge("siwe");
      const { message, signature } = await siweSignIn(challenge);
      const result = await api.verify({
        method: "siwe",
        nonce: challenge.nonce,
        message,
        signature,
      });
      onSignedIn(result.session, result.token);

      // The wallet signature proved who you are and nothing more. The console's
      // own actions are sent from a smart wallet controlled by a Signet key, so
      // signing in has to produce one — otherwise a wallet user could sign in
      // and then find they could not create anything.
      //
      // This is the only MetaMask prompt in the product. Everything after it is
      // signed by the Signet key issued here.
      if (!network) {
        // Skipping this quietly would sign someone in with no key and no way
        // to get one, which is the dead end this whole flow exists to avoid.
        throw new Error(
          "Signed in, but the network configuration has not loaded, so no Signet key could be issued. Reload and try again.",
        );
      }
      setProvisioning(PROVISION_STAGE_COPY.session);
      await provisionSignetKey(network, (s) => setProvisioning(PROVISION_STAGE_COPY[s]));
      router.replace(next);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(null);
      setProvisioning(null);
    }
  }, [network, next, onSignedIn, router]);

  const hasWallet = typeof window !== "undefined" && injectedProvider() !== null;
  const siweEnabled = network?.siwe_enabled ?? false;

  return (
    <main className="grid min-h-screen lg:grid-cols-2">
      {/* Left: the argument, so the sign-in screen still says what this is. */}
      <div className="relative hidden overflow-hidden bg-primary-950 p-12 text-primary-100 lg:flex lg:flex-col lg:justify-between">
        <div
          aria-hidden="true"
          className="anim-aurora pointer-events-none absolute left-1/2 top-1/3 h-[460px] w-[560px] -translate-x-1/2 rounded-full opacity-25 blur-[110px]"
          style={{ background: "radial-gradient(circle, #e8873c 0%, transparent 70%)" }}
        />
        <Link href="/" className="relative inline-flex items-center gap-3 text-primary-50">
          <Logo size={30} />
          <span className="text-[17px] font-semibold tracking-tight">Signet</span>
        </Link>

        <div className="relative max-w-md">
          <h1 className="text-[2.1rem] font-semibold leading-tight tracking-[-0.03em]">
            The console signs you in the same way your app will sign in your users.
          </h1>
          <p className="mt-5 text-[15px] leading-relaxed text-primary-300">
            Social login proves your credential to the bootstrap signing group in zero knowledge.
            The group threshold-signs a challenge. We verify that signature — not a password, not a
            session cookie you had to trust us with.
          </p>
        </div>

        <p className="relative text-[12.5px] text-primary-400">
          You are about to use the product before you build with it.
        </p>
      </div>

      {/* Right: the actual sign-in. */}
      <div className="flex items-center justify-center px-6 py-16">
        <div className="w-full max-w-sm">
          <Link href="/" className="mb-8 inline-flex items-center gap-2.5 text-fg lg:hidden">
            <Logo size={26} />
            <span className="text-[16px] font-semibold tracking-tight">Signet</span>
          </Link>

          <h2 className="text-[22px] font-semibold tracking-tight text-fg">Sign in</h2>
          <p className="mt-2 text-sm leading-relaxed text-muted">
            No password to set, and nothing replayable stored anywhere.
          </p>

          <div className="mt-8 space-y-3">
            <button
              type="button"
              onClick={signInWithSignet}
              disabled={busy !== null}
              className="btn-accent w-full py-3"
            >
              {busy === "signet" ? "Starting…" : "Continue with Signet"}
            </button>

            {siweEnabled ? (
              <button
                type="button"
                onClick={signInWithWallet}
                disabled={busy !== null}
                className="btn-ghost w-full py-3"
              >
                {busy === "siwe"
                  ? (provisioning ?? "Waiting for your wallet…")
                  : "Sign in with Ethereum"}
              </button>
            ) : null}
          </div>

          {siweEnabled && !hasWallet ? (
            <p className="mt-3 text-[12.5px] text-faint">
              Signing in with Ethereum needs a wallet extension in this browser.
            </p>
          ) : null}

          {error ? (
            <div className="mt-5">
              <ErrorNote error={error} />
            </div>
          ) : null}

          <div className="mt-8">
            <Callout title="Which route should I use?">
              <p>
                <strong>Signet</strong> is the dogfooded route and the one your own users will
                take. <strong>Sign in with Ethereum</strong> proves control of an EOA — useful for
                node operators, and for teams who have not onboarded through Signet yet. They are
                separate identities: proving control of an EOA is not proof of control of a Signet
                account, so the console treats them as different people.
              </p>
            </Callout>
          </div>

          <p className="mt-8 text-[12.5px] leading-relaxed text-faint">
            By signing in you agree that this is pre-release software running against a testnet.
            Read the{" "}
            <Link href="/docs" className="underline underline-offset-2 hover:text-muted">
              documentation
            </Link>{" "}
            before pointing production traffic at it.
          </p>
        </div>
      </div>
    </main>
  );
}
