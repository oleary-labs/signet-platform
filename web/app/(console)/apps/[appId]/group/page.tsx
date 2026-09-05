"use client";

import Link from "next/link";
import { use, useCallback, useMemo, useState } from "react";
import { NodeCard } from "@/components/NodeCard";
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
  Skeleton,
  StatCard,
  StatusDot,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useAction, useQuery, useTicker } from "@/lib/hooks";
import { useSession } from "@/providers/SessionProvider";
import { useToast } from "@/providers/ToastProvider";
import { signGroupCall, useCanSignOnchain, type GroupFunction } from "@/lib/onchain";
import { SigningSessionNotice } from "@/components/console/SigningSessionNotice";
import { chainName, countdown, formatDateTime, shortAddress } from "@/lib/format";
import type { GroupNode } from "@/lib/types";

export default function GroupPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);
  const toast = useToast();
  const { network, user } = useSession();
  useTicker(1000); // keep the timelock countdowns ticking

  const group = useQuery(() => api.group(appId), [appId]);
  const operators = useQuery(() => api.nodeOperators(), []);

  const [inviteOpen, setInviteOpen] = useState(false);
  const [attachOpen, setAttachOpen] = useState(false);
  const [pendingAction, setPendingAction] = useState<string | null>(null);

  const app = group.data?.app;
  const onchain = group.data?.onchain;
  const nodes = useMemo(() => group.data?.nodes ?? [], [group.data]);
  const canSign = useCanSignOnchain(user);

  // Every action on this screen is the same shape: sign the call with the
  // Signet key that manages this group, hand it to the platform, and re-read
  // the contract afterwards so the screen shows the chain rather than what it
  // assumed the chain would say.
  const call = useCallback(
    async (label: string, functionName: GroupFunction, args: unknown[]) => {
      if (!network || !user || !app?.group_address) return;
      setPendingAction(label);
      try {
        const userOp = await signGroupCall({
          network,
          user,
          groupAddress: app.group_address,
          functionName,
          args,
          sponsored: app.environment === "development",
        });
        if (!userOp) throw new Error("This app has no signing group yet.");
        await api.executeGroupCall(appId, userOp, functionName);
        await group.refresh();
        toast.success(`${label} confirmed`);
      } catch (err) {
        toast.error(`${label} failed`, err instanceof Error ? err.message : String(err));
      } finally {
        setPendingAction(null);
      }
    },
    [network, user, app?.group_address, app?.environment, appId, group, toast],
  );

  const [sync, { pending: syncing }] = useAction(async () => {
    await api.syncGroup(appId);
    await group.refresh();
    toast.success("Synced from the chain");
  });

  if (group.error) return <ErrorNote error={group.error} onRetry={group.refresh} />;
  if (!app) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-9 w-64" />
        <Skeleton className="h-40 w-full rounded-2xl" />
      </div>
    );
  }

  if (!app.group_address) {
    return (
      <>
        <PageHeader
          title="Signing group"
          info="Every change here is an on-chain transaction from your own account. The platform reads your group and caches it; it cannot alter it."
        />
        <EmptyState
          title="No signing group attached"
          action={
            <div className="flex flex-wrap justify-center gap-2">
              <Link href="/apps/new" className="btn-accent">
                Deploy a group
              </Link>
              <button type="button" className="btn-ghost" onClick={() => setAttachOpen(true)}>
                Link an existing group
              </button>
            </div>
          }
        />
        <AttachModal
          open={attachOpen}
          onClose={() => setAttachOpen(false)}
          appId={appId}
          onDone={() => {
            setAttachOpen(false);
            group.refresh();
          }}
        />
      </>
    );
  }

  const active = nodes.filter((n) => n.status === "active");
  const pending = nodes.filter((n) => n.status === "pending");
  const removing = nodes.filter((n) => n.status === "removing");
  const threshold = onchain?.threshold ?? app.threshold ?? 0;
  const operational = onchain?.is_operational ?? app.is_operational;

  const inGroup = new Set(nodes.map((n) => n.address.toLowerCase()));
  const invitable = (operators.data ?? []).filter((o) => !inGroup.has(o.address.toLowerCase()));

  return (
    <>
      <PageHeader
        title="Signing group"
        info="Every change here is an on-chain transaction from your own account. The platform reads your group and caches it; it cannot alter it."
        actions={
          <button type="button" className="btn-ghost" disabled={syncing} onClick={() => sync()}>
            {syncing ? "Syncing…" : "Sync from chain"}
          </button>
        }
      />

      {/* Every action on this screen is an on-chain transaction from the
          developer's own account. When this session cannot make one, say why —
          a row of silently disabled buttons is the worst possible answer. */}
      {!canSign ? <SigningSessionNotice what="change the group" /> : null}

      {group.data?.sync_error ? (
        <div className="mb-5">
          <Callout
            tone="warn"
            title={
              <>
                Showing the last state the platform could read
                <InfoTip>
                  The chain could not be read just now, so the membership below may be stale. The
                  group itself is unaffected — this is a platform-side read failure.
                </InfoTip>
              </>
            }
          >
            <code className="block break-words">{group.data.sync_error}</code>
          </Callout>
        </div>
      ) : null}

      <div className="mb-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="Configuration"
          value={<span className="mono text-[24px]">{threshold}-of-{active.length}</span>}
          hint={`Any ${threshold} of the ${active.length} active operators can sign.`}
        />
        <StatCard
          label="Status"
          tone={operational ? "success" : "error"}
          value={
            <span className="inline-flex items-center gap-2">
              <StatusDot tone={operational ? "success" : "error"} pulse={!!operational} />
              {operational ? "Operational" : "Below threshold"}
            </span>
          }
          hint={
            operational
              ? `${active.length - threshold} operator${active.length - threshold === 1 ? "" : "s"} can be offline.`
              : "Not enough active operators to produce a signature."
          }
        />
        <StatCard
          label="Removal timelock"
          value={onchain ? formatDelay(onchain.removal_delay) : "—"}
          hint="How long a queued removal waits before anyone can execute it."
        />
        <StatCard
          label="Pending changes"
          tone={pending.length + removing.length > 0 ? "warn" : undefined}
          value={pending.length + removing.length}
          hint="Invitations awaiting acceptance, plus queued removals."
        />
      </div>

      {!operational ? (
        <div className="mb-6">
          <Callout
            tone="error"
            title={
              <>
                Below threshold — {active.length} of {threshold} operators active
                <InfoTip>
                  Your users cannot generate keys or sign until at least {threshold} are active.
                  Invite another operator, or ask the pending ones to accept.
                </InfoTip>
              </>
            }
          />
        </div>
      ) : null}

      <div className="space-y-5">
        <Section
          title={`Active operators (${active.length})`}
          actions={
            <button
              type="button"
              className="btn-accent btn-sm"
              onClick={() => setInviteOpen(true)}
              disabled={!canSign}
            >
              Invite an operator
            </button>
          }
        >
          {active.length === 0 ? (
            <EmptyState title="No active operators" />
          ) : (
            <ul className="space-y-2.5">
              {active.map((n) => (
                <NodeRow
                  key={n.address}
                  node={n}
                  action={
                    <button
                      type="button"
                      className="btn-danger btn-sm"
                      disabled={pendingAction !== null || !canSign}
                      onClick={() => call("Queue removal", "queueRemoval", [n.address])}
                    >
                      {pendingAction === "Queue removal" ? "…" : "Queue removal"}
                    </button>
                  }
                />
              ))}
            </ul>
          )}
        </Section>

        {pending.length > 0 ? (
          <Section
            title={`Awaiting acceptance (${pending.length})`}
          >
            <ul className="space-y-2.5">
              {pending.map((n) => (
                <NodeRow key={n.address} node={n} />
              ))}
            </ul>
          </Section>
        ) : null}

        {removing.length > 0 ? (
          <Section
            title={`Queued removals (${removing.length})`}
          >
            <ul className="space-y-2.5">
              {removing.map((n) => {
                const remaining = countdown(n.execute_after);
                return (
                  <NodeRow
                    key={n.address}
                    node={n}
                    action={
                      <div className="flex flex-none items-center gap-2">
                        {remaining ? (
                          <span className="mono text-accent-600 dark:text-accent-400">
                            {remaining}
                          </span>
                        ) : null}
                        <button
                          type="button"
                          className="btn-ghost btn-sm"
                          disabled={pendingAction !== null}
                          onClick={() => call("Cancel removal", "cancelRemoval", [n.address])}
                        >
                          Cancel
                        </button>
                        <button
                          type="button"
                          className="btn-danger btn-sm"
                          disabled={pendingAction !== null || remaining !== null}
                          title={remaining ? `Executable in ${remaining}` : undefined}
                          onClick={() => call("Execute removal", "executeRemoval", [n.address])}
                        >
                          Execute
                        </button>
                      </div>
                    }
                  />
                );
              })}
            </ul>
          </Section>
        ) : null}

        <Section
          title="Refresh key shares"
        >
          <div className="flex flex-wrap items-center gap-3">
            <button
              type="button"
              className="btn-ghost"
              disabled={pendingAction !== null || !canSign}
              onClick={() => call("Reshare request", "requestReshare", [])}
            >
              {pendingAction === "Reshare request" ? "Requesting…" : "Request a reshare"}
            </button>
            <p className="text-[13px] leading-relaxed text-muted">
              Worth doing after an operator leaves, or on a schedule if you want stolen shares to
              expire on their own.
            </p>
          </div>
        </Section>

        <Section title="Group details">
          <dl className="grid gap-x-8 gap-y-4 sm:grid-cols-2">
            <Field label="Group address">
              <CopyValue
                value={app.group_address}
                display={shortAddress(app.group_address, 12, 8)}
                label="Group address"
              />
            </Field>
            <Field label="Manager">
              {onchain ? (
                <CopyValue value={onchain.manager} display={shortAddress(onchain.manager, 10, 8)} />
              ) : (
                "—"
              )}
            </Field>
            <Field label="Chain">{chainName(app.chain_id)}</Field>
            <Field label="Deployed">{formatDateTime(app.deployed_at)}</Field>
            <Field label="Trusted issuers">
              {onchain?.issuers.length
                ? onchain.issuers.map((i) => i.issuer).join(", ")
                : "None configured"}
            </Field>
            <Field label="Authorization keys">
              {onchain?.auth_keys.length ?? 0} registered
            </Field>
          </dl>
        </Section>
      </div>

      <Modal
        open={inviteOpen}
        onClose={() => setInviteOpen(false)}
        title="Invite an operator"
        wide
        info="Open operators join immediately. Others enter a pending state until they accept."
      >
        {invitable.length === 0 ? (
          <EmptyState
            title="Every listed operator is already in this group"
          />
        ) : (
          <div className="grid max-h-[52vh] gap-3 overflow-y-auto sm:grid-cols-2">
            {invitable.map((o) => (
              <NodeCard
                key={o.address}
                operator={o}
                onToggle={() => {
                  setInviteOpen(false);
                  call("Invite", "inviteNode", [o.address]);
                }}
              />
            ))}
          </div>
        )}
      </Modal>

      <AttachModal
        open={attachOpen}
        onClose={() => setAttachOpen(false)}
        appId={appId}
        onDone={() => {
          setAttachOpen(false);
          group.refresh();
        }}
      />
    </>
  );
}

function NodeRow({ node, action }: { node: GroupNode; action?: React.ReactNode }) {
  const health = node.operator?.health;
  return (
    <li className="flex flex-wrap items-center justify-between gap-3 rounded-xl border px-4 py-3 hairline">
      <div className="flex min-w-0 items-center gap-3">
        <StatusDot
          tone={
            node.status === "removing"
              ? "error"
              : node.status === "pending"
                ? "warn"
                : health?.online
                  ? "success"
                  : "neutral"
          }
          pulse={node.status === "active" && health?.online}
        />
        <div className="min-w-0">
          <p className="truncate text-[13.5px] font-medium text-fg">
            {node.operator?.name ?? "Unlisted operator"}
          </p>
          <p className="mono truncate text-faint">{shortAddress(node.address, 12, 8)}</p>
        </div>
      </div>
      <div className="flex flex-none items-center gap-3">
        {node.operator?.jurisdiction ? (
          <span className="hidden text-[12.5px] text-muted sm:inline">
            {node.operator.jurisdiction}
          </span>
        ) : null}
        {health ? (
          <Badge tone={health.online ? "success" : "error"}>
            {health.online ? `${health.latency_ms ?? "—"} ms` : "unreachable"}
          </Badge>
        ) : (
          <Badge tone="neutral">no probe</Badge>
        )}
        {action}
      </div>
    </li>
  );
}

function AttachModal({
  open,
  onClose,
  appId,
  onDone,
}: {
  open: boolean;
  onClose: () => void;
  appId: string;
  onDone: () => void;
}) {
  const toast = useToast();
  const [address, setAddress] = useState("");

  const [attach, { pending, error }] = useAction(async () => {
    const result = await api.attachGroup(appId, address.trim());
    if (result.sync_error) {
      toast.info("Group linked", `Membership will fill in on the next sync: ${result.sync_error}`);
    } else {
      toast.success("Group linked");
    }
    setAddress("");
    onDone();
  });

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Link an existing signing group"
      info="The platform reads the contract and refuses the link unless the account you signed in with is its manager."
      footer={
        <>
          <button type="button" className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            type="button"
            className="btn-accent"
            disabled={pending || !address.trim()}
            onClick={() => attach()}
          >
            {pending ? "Verifying…" : "Link group"}
          </button>
        </>
      }
    >
      {error ? <ErrorNote error={error} /> : null}
      <label className="label mt-4" htmlFor="group-address">
        Group address
      </label>
      <input
        id="group-address"
        className="input"
        placeholder="0x…"
        value={address}
        onChange={(e) => setAddress(e.target.value)}
      />
    </Modal>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">{label}</dt>
      <dd className="mt-1 text-[13.5px] text-fg">{children}</dd>
    </div>
  );
}

function formatDelay(seconds: number): string {
  if (seconds === 0) return "None";
  if (seconds < 3600) return `${Math.round(seconds / 60)} min`;
  if (seconds < 86_400) return `${Math.round(seconds / 3600)} h`;
  return `${Math.round(seconds / 86_400)} d`;
}
