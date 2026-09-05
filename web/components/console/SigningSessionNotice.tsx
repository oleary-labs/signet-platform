"use client";

import { useState } from "react";
import Link from "next/link";
import { Callout, ErrorNote, InfoTip } from "@/components/ui";
import {
  PROVISION_STAGE_COPY,
  canRestoreSigningSession,
  restoreSigningSession,
} from "@/lib/provision";
import { useSession } from "@/providers/SessionProvider";

/**
 * Shown when a screen's on-chain actions are unavailable because this tab has
 * no signing session.
 *
 * A row of disabled buttons with no explanation is the worst version of this,
 * and "sign in again" is the wrong answer for most people who hit it: a wallet
 * user's key can be recovered from the session they already have, without
 * touching their wallet. So the notice says which case they are in and, where
 * it can, just fixes it.
 */
export function SigningSessionNotice({ what }: { what: string }) {
  const { user, network, refresh } = useSession();
  const [stage, setStage] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);

  const canRestore = canRestoreSigningSession(user);

  async function restore() {
    if (!network || !user) return;
    setError(null);
    try {
      await restoreSigningSession(network, user, (s) => setStage(PROVISION_STAGE_COPY[s]));
      await refresh();
    } catch (err) {
      setError(err);
    } finally {
      setStage(null);
    }
  }

  return (
    <div className="mb-5 space-y-3">
      <Callout
        tone="warn"
        title={
          <>
            No signing key in this tab
            <InfoTip>
              The key that signs on-chain actions is held in this tab only and is never persisted.
              {canRestore
                ? " Your operators can issue it again from the session you already have — no wallet prompt."
                : " It is bound to the credential you signed in with, so getting it back means signing in again."}
            </InfoTip>
          </>
        }
      >
        {canRestore ? (
          <button
            type="button"
            className="btn-accent btn-sm"
            disabled={stage !== null}
            onClick={restore}
          >
            {stage ?? "Restore signing"}
          </button>
        ) : (
          <Link href="/login" className="btn-accent btn-sm">
            Sign in again
          </Link>
        )}
      </Callout>
      {error ? <ErrorNote error={error} /> : null}
    </div>
  );
}
