"use client";

import { Callout, InfoTip } from "@/components/ui";
import { SigningSessionNotice } from "./SigningSessionNotice";
import { shortAddress } from "@/lib/format";
import type { Transport } from "@/lib/onchain";

/**
 * Says which key is about to sign, before it does.
 *
 * A group is managed either by the Signet wallet that created it or by an EOA
 * — the developer's own, once they have taken it over, or from the start if
 * the group was created out of band. Those differ in ways the developer feels:
 * one is sponsored and invisible, the other opens their wallet and spends
 * their gas. Deciding silently would be defensible right up until someone
 * wonders why a button sometimes costs money.
 *
 * It also replaces a wrong message. Screens used to show "this tab has no
 * signing session" whenever a Signet key was missing, which for a developer
 * managing an EOA-held group is both alarming and false — they never needed
 * one.
 */
export function TransportNotice({ transport, what }: { transport: Transport; what: string }) {
  if (transport.kind === "userop") {
    return (
      <Callout
        tone="accent"
        title={
          <>
            Signed by your Signet key
            <InfoTip>
              A quorum of your operators signs the operation and the platform relays it, so
              nothing opens and no gas leaves your wallet.
            </InfoTip>
          </>
        }
      />
    );
  }

  if (transport.kind === "wallet") {
    return (
      <Callout
        tone="warn"
        title={
          <>
            Signed by your wallet — {shortAddress(transport.from, 6, 4)}
            <InfoTip>
              This group is managed by an address you hold directly, so the change goes out as an
              ordinary transaction from that account. Your wallet will ask you to confirm it, and
              you pay the gas. The platform records the result once it is mined.
            </InfoTip>
          </>
        }
      />
    );
  }

  // A missing signing session is recoverable without touching a wallet, and
  // SigningSessionNotice already knows how — so hand that case over rather
  // than restating it as a dead end.
  if (transport.reason.includes("signing session")) {
    return <SigningSessionNotice what={what} />;
  }

  return <Callout tone="warn" title="You cannot change this group">{transport.reason}</Callout>;
}
