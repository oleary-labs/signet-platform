"use client";

import { use, useState } from "react";
import {
  Badge,
  Callout,
  ErrorNote,
  InfoTip,
  Modal,
  PageHeader,
  Section,
  SkeletonRows,
  Toggle,
} from "@/components/ui";
import { SettingsForm } from "@/components/console/SettingsForm";
import { api } from "@/lib/api";
import { useAction, useQuery } from "@/lib/hooks";
import { signGroupCall } from "@/lib/onchain";
import { USEROP_STAGE_COPY, type UserOpStage } from "@/lib/userop";
import { useSession } from "@/providers/SessionProvider";
import { useToast } from "@/providers/ToastProvider";
import type { LoginMethodSettings } from "@/lib/types";

/**
 * Login methods.
 *
 * Two lists that look similar and are not. The issuers at the top are written
 * to the group contract and checked by every operator independently — they are
 * what actually decides whose credential can open a session. The toggles below
 * only decide what your login modal offers; turning one on without a matching
 * issuer would show your users a button that cannot work, which is why the
 * screen says which is which.
 */
const CHAIN_ENFORCED = [
  {
    key: "auth_key" as const,
    label: "Authorization key certificates",
    description:
      "Server-to-server sessions signed by a key your group trusts. This is how your own backend authenticates.",
  },
  {
    key: "onchain_resolver" as const,
    label: "On-chain identity resolver (SIWE)",
    description:
      "Gate sessions on a contract your group is bound to — an allowlist, an ERC-8004 registry, a soulbound token.",
  },
];

const CLIENT_METHODS = [
  { key: "email" as const, label: "Email", description: "One-time codes to an email address." },
  { key: "sms" as const, label: "SMS", description: "One-time codes to a phone number." },
  { key: "passkey" as const, label: "Passkeys", description: "WebAuthn, on the user's device." },
  { key: "wallet" as const, label: "External wallet", description: "MetaMask, Rainbow, WalletConnect." },
  { key: "farcaster" as const, label: "Farcaster", description: "Sign in with a Farcaster account." },
  { key: "telegram" as const, label: "Telegram", description: "Sign in with a Telegram account." },
  { key: "guest" as const, label: "Guest accounts", description: "A wallet before the user picks an identity." },
];

export default function LoginMethodsPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);
  const toast = useToast();
  const { user, network } = useSession();
  const app = useQuery(() => api.app(appId), [appId]);
  const issuers = useQuery(() => api.issuers(appId), [appId]);
  const [addOpen, setAddOpen] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);

  // Removing an issuer is an on-chain call once the group exists, so the
  // console signs it here and the platform forwards it. Removing it from the
  // database alone would leave every operator still trusting it.
  const [remove, { error: removeError }] = useAction(async (issuer: { id: string; issuer: string }) => {
    setBusy(issuer.id);
    try {
      const { keccak256, toBytes } = await import("viem");
      const userOp =
        user && network
          ? await signGroupCall({
              network,
              user,
              groupAddress: app.data?.group_address,
              functionName: "removeIssuer",
              args: [keccak256(toBytes(issuer.issuer))],
              sponsored: app.data?.environment === "development",
            })
          : undefined;
      const result = await api.removeIssuer(appId, issuer.id, userOp);
      await issuers.refresh();
      toast.success(
        "Removed",
        result.transaction_hash ? "Your operators no longer trust it." : undefined,
      );
    } finally {
      setBusy(null);
    }
  });

  return (
    <>
      <PageHeader
        title="Login methods"
        info="An empty client-ID list accepts any client from that issuer. A list containing an empty string matches nothing — narrow it to your own OAuth client before launch."
      />

      <div className="space-y-5">
        <Section
          title="Trusted issuers"
          actions={
            <button type="button" className="btn-accent btn-sm" onClick={() => setAddOpen(true)}>
              Add an issuer
            </button>
          }
        >
          {issuers.error ? <ErrorNote error={issuers.error} onRetry={issuers.refresh} /> : null}
          {removeError ? <ErrorNote error={removeError} /> : null}
          {issuers.loading ? (
            <SkeletonRows rows={2} />
          ) : (issuers.data ?? []).length === 0 ? (
            <Callout
              tone="warn"
              title={
                <>
                  No issuers configured
                  <InfoTip>
                    Without a trusted issuer your users cannot open a session at all. Add the OAuth
                    provider they will sign in with.
                  </InfoTip>
                </>
              }
            />
          ) : (
            <ul className="space-y-2.5">
              {issuers.data!.map((i) => (
                <li
                  key={i.id}
                  className="flex flex-wrap items-center justify-between gap-3 rounded-xl border px-4 py-3 hairline"
                >
                  <div className="min-w-0">
                    <p className="text-[13.5px] font-medium text-fg">{i.label || i.provider}</p>
                    <p className="mono truncate text-faint">{i.issuer}</p>
                    <p className="mt-1 text-[12px] text-muted">
                      {i.client_ids.length === 0
                        ? "Any client from this issuer is accepted"
                        : `${i.client_ids.length} client ID${i.client_ids.length === 1 ? "" : "s"} allowed`}
                    </p>
                  </div>
                  <div className="flex flex-none items-center gap-3">
                    <Badge
                      tone={
                        i.onchain_status === "active"
                          ? "success"
                          : i.onchain_status === "removed"
                            ? "error"
                            : "warn"
                      }
                      dot
                    >
                      {i.onchain_status === "active" ? "trusted on-chain" : i.onchain_status}
                    </Badge>
                    <button
                      type="button"
                      className="btn-danger btn-sm"
                      disabled={busy === i.id}
                      onClick={() => remove({ id: i.id, issuer: i.issuer })}
                    >
                      {busy === i.id ? "Removing…" : "Remove"}
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}

        </Section>

        <SettingsForm<"login_methods">
          appId={appId}
          section="login_methods"
          defaults={{} as LoginMethodSettings}
        >
          {(value, set) => (
            <>
              <Section
                title="Enforced by your operators"
              >
                <div className="space-y-2.5">
                  {CHAIN_ENFORCED.map((m) => (
                    <Toggle
                      key={m.key}
                      checked={value[m.key] ?? false}
                      onChange={(v) => set({ [m.key]: v } as Partial<LoginMethodSettings>)}
                      label={m.label}
                    />
                  ))}
                </div>
              </Section>

              <Section
                title="Offered in your login modal"
              >
                <div className="space-y-2.5">
                  {CLIENT_METHODS.map((m) => (
                    <Toggle
                      key={m.key}
                      checked={value[m.key] ?? false}
                      onChange={(v) => set({ [m.key]: v } as Partial<LoginMethodSettings>)}
                      label={m.label}
                    />
                  ))}
                </div>

                <Callout
                  tone="warn"
                  title={
                    <>
                      Not all of these are enforceable yet
                      <InfoTip>
                        Email and SMS codes, passkeys, Farcaster, Telegram, and guest accounts need
                        an issuer the protocol can verify in zero knowledge — today, an OIDC
                        provider with an RSA-2048 JWKS. Enabling one records your intent and drives
                        your own login UI; it does not make the nodes accept a credential they
                        cannot verify. See FEATURE_EXPANSION.md for what each one needs.
                      </InfoTip>
                    </>
                  }
                />
              </Section>
            </>
          )}
        </SettingsForm>
      </div>

      <AddIssuerModal
        open={addOpen}
        onClose={() => setAddOpen(false)}
        appId={appId}
        groupAddress={app.data?.group_address ?? null}
        sponsored={app.data?.environment === "development"}
        onDone={() => {
          setAddOpen(false);
          issuers.refresh();
        }}
      />
    </>
  );
}

function AddIssuerModal({
  open,
  onClose,
  appId,
  groupAddress,
  sponsored,
  onDone,
}: {
  open: boolean;
  onClose: () => void;
  appId: string;
  groupAddress: string | null;
  sponsored: boolean;
  onDone: () => void;
}) {
  const toast = useToast();
  const { user, network } = useSession();
  const [issuer, setIssuer] = useState("");
  const [clientIds, setClientIds] = useState("");
  const [stage, setStage] = useState<string | null>(null);

  const [add, { pending, error }] = useAction(async () => {
    const trimmed = issuer.trim();
    const ids = clientIds
      .split(",")
      .map((c) => c.trim())
      .filter(Boolean);

    try {
      // Before the group exists there is nothing to call: the issuer is
      // written into createGroup instead, so it goes on-chain either way.
      const userOp =
        user && network
          ? await signGroupCall({
              network,
              user,
              groupAddress,
              functionName: "addIssuer",
              args: [trimmed, ids],
              sponsored,
              onStage: (s) => setStage(s),
            })
          : undefined;

      const result = await api.saveIssuer(appId, {
        issuer: trimmed,
        client_ids: ids,
        user_op: userOp,
      });
      toast.success(
        "Issuer added",
        result.transaction_hash
          ? "Your operators trust it now."
          : "It will be written to your group when you deploy it.",
      );
      setIssuer("");
      setClientIds("");
      onDone();
    } finally {
      setStage(null);
    }
  });

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Add a trusted issuer"
      info="The issuer URL exactly as it appears in the `iss` claim of the tokens your users will present."
      footer={
        <>
          <button type="button" className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            type="button"
            className="btn-accent"
            disabled={pending || !issuer.trim()}
            onClick={() => add()}
          >
            {pending ? (stage ? `${USEROP_STAGE_COPY[stage as UserOpStage]}…` : "Saving…") : "Save issuer"}
          </button>
        </>
      }
    >
      {error ? <ErrorNote error={error} /> : null}
      <label className="label mt-4" htmlFor="issuer">
        Issuer URL
      </label>
      <input
        id="issuer"
        className="input"
        placeholder="https://accounts.google.com"
        value={issuer}
        onChange={(e) => setIssuer(e.target.value)}
      />
      <label className="label mt-4" htmlFor="clients">
        Allowed client IDs
      </label>
      <input
        id="clients"
        className="input"
        placeholder="123-abc.apps.googleusercontent.com"
        value={clientIds}
        onChange={(e) => setClientIds(e.target.value)}
      />
    </Modal>
  );
}
