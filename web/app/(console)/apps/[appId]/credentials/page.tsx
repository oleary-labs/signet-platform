"use client";

import { use, useState } from "react";
import {
  Badge,
  Callout,
  CopyValue,
  EmptyState,
  ErrorNote,
  InfoTip,
  Modal,
  PageHeader,
  Section,
  SkeletonRows,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useAction, useQuery } from "@/lib/hooks";
import { useSession } from "@/providers/SessionProvider";
import { useToast } from "@/providers/ToastProvider";
import { signGroupCall, useCanSignOnchain } from "@/lib/onchain";
import { SigningSessionNotice } from "@/components/console/SigningSessionNotice";
import { USEROP_STAGE_COPY, type UserOpStage } from "@/lib/userop";
import { formatDateTime, relativeTime, shortHash } from "@/lib/format";
import type { Credential } from "@/lib/types";

export default function CredentialsPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);
  const toast = useToast();
  const { network, user } = useSession();

  const creds = useQuery(() => api.credentials(appId), [appId]);
  const group = useQuery(() => api.group(appId), [appId]);

  const [secretOpen, setSecretOpen] = useState(false);
  const [authKeyOpen, setAuthKeyOpen] = useState(false);
  const [revealed, setRevealed] = useState<{ title: string; value: string; note: string } | null>(null);

  const canWriteChain = useCanSignOnchain(user);
  const secrets = (creds.data ?? []).filter((c) => c.kind === "app_secret");
  const authKeys = (creds.data ?? []).filter((c) => c.kind === "auth_key");

  // An authorization key is revoked in both places at once. Removing only the
  // platform's record would leave every operator still accepting it while the
  // console showed it gone.
  const [revoke, { error: revokeError }] = useAction(async (c: Credential) => {
    const userOp =
      c.kind === "auth_key" && c.onchain_status === "active" && c.key_hash && network && user
        ? await signGroupCall({
            network,
            user,
            groupAddress: group.data?.app.group_address ?? null,
            functionName: "removeAuthKey",
            args: [c.key_hash as `0x${string}`],
            sponsored: group.data?.app.environment === "development",
          })
        : undefined;
    const result = await api.revokeCredential(appId, c.id, userOp);
    await creds.refresh();
    toast.success(
      "Revoked",
      result.transaction_hash ? "Your operators no longer accept it." : undefined,
    );
  });

  return (
    <>
      <PageHeader
        title="Keys & credentials"
        info="An authorization key is registered with your operators when you add it and removed when you revoke it. App secrets live only here."
      />

      {!canWriteChain ? <SigningSessionNotice what="register a key with your group" /> : null}

      {revokeError ? (
        <div className="mb-5">
          <ErrorNote error={revokeError} />
        </div>
      ) : null}

      <div className="space-y-5">
        <Section
          title="Authorization keys"
          actions={
            <button
              type="button"
              className="btn-accent btn-sm"
              onClick={() => setAuthKeyOpen(true)}
              disabled={!group.data?.app.group_address}
            >
              Generate a key
            </button>
          }
        >
          {creds.error ? <ErrorNote error={creds.error} onRetry={creds.refresh} /> : null}
          {creds.loading ? (
            <SkeletonRows rows={2} />
          ) : authKeys.length === 0 ? (
            <EmptyState
              title="No authorization keys"
              action={
                <button
                  type="button"
                  className="btn-accent"
                  onClick={() => setAuthKeyOpen(true)}
                  disabled={!group.data?.app.group_address}
                >
                  Generate a key
                </button>
              }
            />
          ) : (
            <ul className="space-y-2.5">
              {authKeys.map((c) => (
                <li
                  key={c.id}
                  className="flex flex-wrap items-center justify-between gap-3 rounded-xl border px-4 py-3 hairline"
                >
                  <div className="min-w-0">
                    <p className="text-[13.5px] font-medium text-fg">
                      {c.label || "Application key"}
                    </p>
                    <div className="mt-0.5 min-w-0">
                      <CopyValue
                        value={c.public_key ?? ""}
                        display={shortHash(c.public_key ?? "", 16, 10)}
                        label="Public key"
                      />
                    </div>
                  </div>
                  <div className="flex flex-none items-center gap-3">
                    <Badge
                      tone={
                        c.onchain_status === "active"
                          ? "success"
                          : c.onchain_status === "removed"
                            ? "error"
                            : "warn"
                      }
                      dot
                    >
                      {c.onchain_status === "active"
                        ? "trusted on-chain"
                        : c.onchain_status === "removed"
                          ? "not on-chain"
                          : c.onchain_status}
                    </Badge>
                    <button type="button" className="btn-danger btn-sm" onClick={() => revoke(c)}>
                      Revoke
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}

        </Section>

        <Section
          title="Application secrets"
          actions={
            <button type="button" className="btn-ghost btn-sm" onClick={() => setSecretOpen(true)}>
              New secret
            </button>
          }
        >
          {creds.loading ? (
            <SkeletonRows rows={2} />
          ) : secrets.length === 0 ? (
            <EmptyState
              title="No application secrets"
            />
          ) : (
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>Label</th>
                    <th>Secret</th>
                    <th>Created</th>
                    <th>Last used</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {secrets.map((c) => (
                    <tr key={c.id}>
                      <td className="text-[13.5px] text-fg">{c.label || "Untitled"}</td>
                      <td className="mono text-muted">sk_signet_…{c.last_four}</td>
                      <td className="whitespace-nowrap text-[13px] text-muted">
                        {formatDateTime(c.created_at)}
                      </td>
                      <td className="whitespace-nowrap text-[13px] text-muted">
                        {c.last_used_at ? relativeTime(c.last_used_at) : "Never"}
                      </td>
                      <td className="text-right">
                        <button type="button" className="btn-danger btn-sm" onClick={() => revoke(c)}>
                          Revoke
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Section>
      </div>

      <NewSecretModal
        open={secretOpen}
        onClose={() => setSecretOpen(false)}
        appId={appId}
        onCreated={(secret, label) => {
          setSecretOpen(false);
          creds.refresh();
          setRevealed({
            title: label || "Application secret",
            value: secret,
            note: "Copy it now. It is stored only as a hash, so this is the only time it can be shown.",
          });
        }}
      />

      <NewAuthKeyModal
        open={authKeyOpen}
        onClose={() => setAuthKeyOpen(false)}
        appId={appId}
        groupAddress={group.data?.app.group_address ?? null}
        sponsored={group.data?.app.environment === "development"}
        canWriteChain={canWriteChain}
        onCreated={(privateKey, label) => {
          setAuthKeyOpen(false);
          creds.refresh();
          setRevealed({
            title: label || "Application key",
            value: privateKey,
            note:
              "This private key was generated in your browser and has never left it. The platform stores only the public half — if you lose this, generate a new key and remove the old one from the group.",
          });
        }}
      />

      <Modal
        open={revealed !== null}
        onClose={() => setRevealed(null)}
        title={revealed?.title ?? ""}
        description="Shown once."
        footer={
          <button type="button" className="btn-accent" onClick={() => setRevealed(null)}>
            I have saved it
          </button>
        }
      >
        <div className="rounded-xl border border-accent-500/40 bg-accent-500/[0.07] p-4">
          <CopyValue value={revealed?.value ?? ""} className="w-full" />
        </div>
        <p className="mt-3 text-[13px] leading-relaxed text-muted">{revealed?.note}</p>
      </Modal>
    </>
  );
}

function NewSecretModal({
  open,
  onClose,
  appId,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  appId: string;
  onCreated: (secret: string, label: string) => void;
}) {
  const [label, setLabel] = useState("");
  const [create, { pending, error }] = useAction(async () => {
    const cred = await api.createCredential(appId, { kind: "app_secret", label });
    onCreated(cred.secret ?? "", label);
    setLabel("");
  });

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="New application secret"
      info="A bearer credential for the platform API. It cannot authorize anything on your signing group."
      footer={
        <>
          <button type="button" className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="button" className="btn-accent" disabled={pending} onClick={() => create()}>
            {pending ? "Creating…" : "Create secret"}
          </button>
        </>
      }
    >
      {error ? <ErrorNote error={error} /> : null}
      <label className="label mt-4" htmlFor="secret-label">
        Label
      </label>
      <input
        id="secret-label"
        className="input"
        placeholder="CI pipeline"
        value={label}
        onChange={(e) => setLabel(e.target.value)}
      />
    </Modal>
  );
}

/**
 * Generating an authorization key.
 *
 * The keypair is generated in the browser with WebCrypto-backed randomness.
 * The private half is shown once and never transmitted — sending it to the
 * platform would make the platform able to authorize signing on the
 * developer's group, which is exactly the concentration of authority the whole
 * system exists to avoid.
 */
function NewAuthKeyModal({
  open,
  onClose,
  appId,
  groupAddress,
  sponsored,
  canWriteChain,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  appId: string;
  groupAddress: string | null;
  sponsored: boolean;
  canWriteChain: boolean;
  onCreated: (privateKey: string, label: string) => void;
}) {
  const { network, user } = useSession();
  const toast = useToast();
  const [label, setLabel] = useState("");
  const [stage, setStage] = useState<string | null>(null);

  const [create, { pending, error }] = useAction(async () => {
    if (!groupAddress || !network || !user) throw new Error("This app has no signing group yet.");

    setStage("Generating a keypair in your browser");
    const { secp256k1 } = await import("@noble/curves/secp256k1");
    const priv = secp256k1.utils.randomPrivateKey();
    const pub = secp256k1.getPublicKey(priv, true);
    const privHex = toHex(priv);
    const pubHex = `0x${toHex(pub)}`;

    // One request: the platform registers the key with the group and records
    // it, or does neither. A key in the console that the operators never heard
    // of is worse than a failure, because it looks like it works.
    const userOp = await signGroupCall({
      network,
      user,
      groupAddress,
      functionName: "addAuthKey",
      args: [pubHex as `0x${string}`],
      sponsored,
      onStage: (s) => setStage(USEROP_STAGE_COPY[s as UserOpStage]),
    });

    setStage("Recording it on the platform");
    await api.createCredential(appId, {
      kind: "auth_key",
      label,
      public_key: pubHex,
      user_op: userOp,
    });

    setStage(null);
    toast.success("Authorization key added to your group");
    onCreated(privHex, label);
    setLabel("");
  });

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Generate an authorization key"
      info="The keypair is created here, in your browser. The public half goes on-chain so your operators trust it; the private half is shown once and never sent anywhere."
      footer={
        <>
          <button type="button" className="btn-ghost" onClick={onClose} disabled={pending}>
            Cancel
          </button>
          <button
            type="button"
            className="btn-accent"
            disabled={pending || !canWriteChain}
            onClick={() => create()}
          >
            {pending ? stage ?? "Working…" : "Generate and register"}
          </button>
        </>
      }
    >
      {error ? <ErrorNote error={error} /> : null}
      {!canWriteChain ? (
        <Callout
          tone="warn"
          title={
            <>
              This session cannot write to the chain
              <InfoTip>
                Registering the key is an on-chain call signed by your Signet key, and this tab has
                none. Restore signing, or add the key from your own tooling and record its public
                half here afterwards.
              </InfoTip>
            </>
          }
        />
      ) : null}

      <label className="label mt-4" htmlFor="key-label">
        Label
      </label>
      <input
        id="key-label"
        className="input"
        placeholder="Backend service"
        value={label}
        onChange={(e) => setLabel(e.target.value)}
      />
    </Modal>
  );
}

function toHex(bytes: Uint8Array): string {
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}
