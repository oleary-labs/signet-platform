"use client";

import Link from "next/link";
import { use, useMemo, useState } from "react";
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
  StatCard,
  Tabs,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useAction, useQuery } from "@/lib/hooks";
import { useSession } from "@/providers/SessionProvider";
import { useToast } from "@/providers/ToastProvider";
import { chainName, curveLabel, formatNumber, relativeTime, scopeLabel, shortAddress, shortHash } from "@/lib/format";
import { loadSigningSession } from "@/lib/session-key";
import type { Key } from "@/lib/types";

export default function WalletsPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);
  const toast = useToast();
  const { network } = useSession();
  const [tab, setTab] = useState("all");
  const [search, setSearch] = useState("");
  const [detail, setDetail] = useState<Key | null>(null);
  const [syncOpen, setSyncOpen] = useState(false);

  const filters = useMemo(() => {
    if (tab === "scoped") return { scope: "eip712" };
    if (tab === "disabled") return { status: "disabled" };
    return {};
  }, [tab]);

  const keys = useQuery(
    () => api.keys(appId, { ...filters, q: search || undefined, limit: "200" }),
    [appId, tab, search],
  );
  const group = useQuery(() => api.group(appId), [appId]);

  const stats = keys.data?.stats;

  const [setStatus, { pending: statusPending }] = useAction(
    async (key: Key, status: "enabled" | "disabled") => {
      // The nodes are authoritative: disable on the group first, then record it
      // here. Recording it first would show a key as disabled while it was
      // still perfectly able to sign.
      const nodeUrl = group.data?.nodes.find((n) => n.operator?.api_url)?.operator?.api_url;
      if (!nodeUrl) {
        throw new Error(
          "No operator in this group publishes an API endpoint, so the console cannot reach the nodes to change key state.",
        );
      }
      const session = loadSigningSession();
      if (!session) {
        throw new Error(
          "This tab has no signing session. Sign in with Signet to change key state — the request has to be signed by your session key, not by the platform.",
        );
      }
      const path = status === "disabled" ? "/v1/keys/disable" : "/v1/keys/enable";
      await api.nodeProxy(nodeUrl, path, {
        group_id: group.data?.app.group_address,
        key_id: key.key_id,
        curve: key.curve,
      });
      await api.updateKey(appId, key.curve, key.key_id, { status });
      await keys.refresh();
      toast.success(status === "disabled" ? "Key disabled" : "Key enabled");
      setDetail(null);
    },
  );

  return (
    <>
      <PageHeader
        title="Wallets"
        info="Syncing needs a credential your group trusts. The platform holds none, so the console fetches the inventory with your own authorization key and caches the result."
        actions={
          <button type="button" className="btn-ghost" onClick={() => setSyncOpen(true)}>
            Sync from nodes
          </button>
        }
      />

      <div className="mb-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard label="Total keys" value={formatNumber(stats?.total ?? 0)} />
        <StatCard label="Enabled" tone="success" value={formatNumber(stats?.enabled ?? 0)} />
        <StatCard
          label="Disabled"
          tone={stats?.disabled ? "warn" : undefined}
          value={formatNumber(stats?.disabled ?? 0)}
          hint="Disabled keys refuse to sign and cannot mint delegations."
        />
        <StatCard
          label="Scoped sub-keys"
          value={formatNumber(
            (stats?.by_scope?.eip712 ?? 0) +
              (stats?.by_scope?.evm_userop ?? 0) +
              (stats?.by_scope?.solana_tx ?? 0),
          )}
          hint="Keys constrained to a specific contract and message type."
        />
      </div>

      <Section>
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <Tabs
            tabs={[
              { id: "all", label: "All keys", count: stats?.total },
              { id: "scoped", label: "Scoped" },
              { id: "disabled", label: "Disabled", count: stats?.disabled },
            ]}
            active={tab}
            onChange={setTab}
          />
          <input
            className="input w-56"
            placeholder="Search key ID or address…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            aria-label="Search keys"
          />
        </div>

        {keys.error ? <ErrorNote error={keys.error} onRetry={keys.refresh} /> : null}

        {keys.loading ? (
          <SkeletonRows rows={6} />
        ) : (keys.data?.keys ?? []).length === 0 ? (
          <EmptyState
            title="No keys here yet"
            action={
              <button type="button" className="btn-accent" onClick={() => setSyncOpen(true)}>
                Sync from nodes
              </button>
            }
          />
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Key</th>
                  <th>Scheme</th>
                  <th>Scope</th>
                  <th>Address</th>
                  <th>Status</th>
                  <th>Synced</th>
                </tr>
              </thead>
              <tbody>
                {keys.data!.keys.map((k) => (
                  <tr
                    key={k.id}
                    className="cursor-pointer"
                    onClick={() => setDetail(k)}
                  >
                    <td className="max-w-[280px]">
                      <p className="truncate text-[13px] font-medium text-fg">
                        {k.label || shortHash(k.key_id, 22, 10)}
                      </p>
                      {k.parent_key_id ? (
                        <p className="mt-0.5 text-[11.5px] text-faint">sub-key</p>
                      ) : null}
                    </td>
                    <td className="whitespace-nowrap text-[13px] text-muted">
                      {curveLabel(k.curve)}
                    </td>
                    <td>
                      {k.scope_kind === "unscoped" ? (
                        <Badge tone="neutral">Unscoped</Badge>
                      ) : (
                        <Badge tone="accent">{scopeLabel(k.scope_kind)}</Badge>
                      )}
                    </td>
                    <td className="mono whitespace-nowrap text-muted">
                      {k.address ? shortAddress(k.address) : "—"}
                    </td>
                    <td>
                      <Badge tone={k.status === "enabled" ? "success" : k.status === "disabled" ? "warn" : "error"} dot>
                        {k.status}
                      </Badge>
                    </td>
                    <td className="whitespace-nowrap text-[13px] text-muted">
                      {relativeTime(k.synced_at)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>

      <KeyDetailModal
        keyRow={detail}
        onClose={() => setDetail(null)}
        onSetStatus={setStatus}
        pending={statusPending}
        chainId={network?.chain_id}
        appId={appId}
      />

      <SyncModal
        open={syncOpen}
        onClose={() => setSyncOpen(false)}
        appId={appId}
        groupAddress={group.data?.app.group_address ?? null}
        nodeUrl={group.data?.nodes.find((n) => n.operator?.api_url)?.operator?.api_url ?? null}
        existingKeyCount={stats?.total ?? 0}
        onDone={() => {
          setSyncOpen(false);
          keys.refresh();
        }}
      />
    </>
  );
}

function KeyDetailModal({
  keyRow,
  onClose,
  onSetStatus,
  pending,
  chainId,
  appId,
}: {
  keyRow: Key | null;
  onClose: () => void;
  onSetStatus: (k: Key, s: "enabled" | "disabled") => Promise<unknown>;
  pending: boolean;
  chainId?: number;
  appId: string;
}) {
  if (!keyRow) return null;
  const k = keyRow;
  return (
    <Modal open onClose={onClose} title={k.label || "Key"} wide>
      <dl className="grid gap-x-8 gap-y-4 sm:grid-cols-2">
        <Field label="Key ID">
          <CopyValue value={k.key_id} display={shortHash(k.key_id, 26, 12)} label="Key ID" />
        </Field>
        <Field label="Scheme">{curveLabel(k.curve)}</Field>
        {k.address ? (
          <Field label="Address">
            <CopyValue value={k.address} display={shortAddress(k.address, 10, 8)} label="Address" />
          </Field>
        ) : null}
        {k.public_key ? (
          <Field label="Public key">
            <CopyValue value={k.public_key} display={shortHash(k.public_key, 14, 8)} label="Public key" />
          </Field>
        ) : null}
        <Field label="Threshold">
          {k.threshold ? `${k.threshold}-of-${k.parties.length || "?"}` : "—"}
        </Field>
        <Field label="Status">
          <Badge tone={k.status === "enabled" ? "success" : "warn"} dot>
            {k.status}
          </Badge>
        </Field>
        {k.parent_key_id ? (
          <Field label="Parent key">
            <CopyValue value={k.parent_key_id} display={shortHash(k.parent_key_id, 22, 10)} />
          </Field>
        ) : null}
        {k.subject_hash ? (
          <Field label="User">
            <CopyValue value={k.subject_hash} display={shortHash(k.subject_hash, 14, 8)} />
          </Field>
        ) : null}
      </dl>

      {k.scope_kind !== "unscoped" ? (
        <div className="mt-6">
          <Callout
            tone="accent"
            title={
              <>
                Scoped: {scopeLabel(k.scope_kind)}
                <InfoTip>
                  This key can only sign the exact payload shape below. Every operator re-checks the
                  scope independently before contributing a share, so the constraint holds even if
                  the node that received the request is malicious.
                </InfoTip>
              </>
            }
          >
            <dl className="grid gap-2 text-[12.5px]">
              {k.scope_chain_id ? (
                <div>
                  <span className="text-faint">Chain: </span>
                  {chainName(k.scope_chain_id)}
                </div>
              ) : null}
              {k.scope_contract ? (
                <div className="min-w-0">
                  <span className="text-faint">Contract: </span>
                  <CopyValue value={k.scope_contract} display={shortAddress(k.scope_contract, 10, 8)} />
                </div>
              ) : null}
              {k.scope_type_hash ? (
                <div className="min-w-0">
                  <span className="text-faint">Type hash: </span>
                  <CopyValue value={k.scope_type_hash} display={shortHash(k.scope_type_hash, 12, 8)} />
                </div>
              ) : null}
            </dl>
          </Callout>
        </div>
      ) : null}

      {k.parties.length > 0 ? (
        <div className="mt-6">
          <p className="label">Shareholders</p>
          <ul className="space-y-1.5">
            {k.parties.map((p) => (
              <li key={p} className="mono truncate text-muted">
                {p}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <div className="mt-7 flex flex-wrap items-center justify-between gap-3 border-t pt-5 hairline">
        <Link href={`/apps/${appId}/configuration/session-signers`} className="btn-quiet btn-sm">
          Session signers for this key →
        </Link>
        {k.status === "enabled" ? (
          <button
            type="button"
            className="btn-danger"
            disabled={pending}
            onClick={() => onSetStatus(k, "disabled")}
          >
            {pending ? "Disabling…" : "Disable key"}
          </button>
        ) : (
          <button
            type="button"
            className="btn-ghost"
            disabled={pending}
            onClick={() => onSetStatus(k, "enabled")}
          >
            {pending ? "Enabling…" : "Enable key"}
          </button>
        )}
      </div>
    </Modal>
  );
}

/**
 * Syncing the key inventory.
 *
 * The platform cannot fetch this itself: POST /admin/keys wants a signature
 * from an authorization key the group trusts, and the platform deliberately
 * holds no such key. So the console fetches it with the developer's own
 * credential — through the node proxy — and posts the result back to cache.
 */
function SyncModal({
  open,
  onClose,
  appId,
  groupAddress,
  nodeUrl,
  existingKeyCount,
  onDone,
}: {
  open: boolean;
  onClose: () => void;
  appId: string;
  groupAddress: string | null;
  nodeUrl: string | null;
  existingKeyCount: number;
  onDone: () => void;
}) {
  const toast = useToast();
  const [raw, setRaw] = useState("");

  const [sync, { pending, error }] = useAction(async () => {
    let keys: unknown[];
    if (raw.trim()) {
      const parsed = JSON.parse(raw);
      keys = Array.isArray(parsed) ? parsed : (parsed.keys ?? []);
    } else {
      if (!nodeUrl || !groupAddress) {
        throw new Error(
          "No operator in this group publishes an API endpoint, so the console cannot fetch the inventory. Paste it below instead.",
        );
      }
      const session = loadSigningSession();
      if (!session) {
        throw new Error(
          "Fetching the inventory needs a credential the group trusts, and this tab has no signing session. Sign in with Signet, or paste the response from your own admin call below.",
        );
      }
      const result = (await api.nodeProxy(nodeUrl, "/admin/keys", {
        group_id: groupAddress,
      })) as unknown;
      keys = Array.isArray(result) ? result : [];
    }

    // Syncing an empty inventory marks every cached key deleted. That is right
    // when the group genuinely holds none, and destructive when the fetch just
    // came back empty — so ask before doing it to a populated list.
    if (keys.length === 0 && existingKeyCount > 0) {
      const confirmed = window.confirm(
        `The inventory came back empty, but this app currently shows ${existingKeyCount} key(s). ` +
          "Continuing will mark all of them deleted. Sync anyway?",
      );
      if (!confirmed) return;
    }

    const { synced } = await api.syncKeys(appId, keys);
    toast.success(`Synced ${synced} key${synced === 1 ? "" : "s"}`);
    setRaw("");
    onDone();
  });

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Sync key inventory"
      wide
      info="The platform holds no credential your nodes would accept — that is deliberate — so the inventory is fetched with your own authorization key and cached here."
      footer={
        <>
          <button type="button" className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="button" className="btn-accent" disabled={pending} onClick={() => sync()}>
            {pending ? "Syncing…" : "Sync"}
          </button>
        </>
      }
    >
      {error ? <ErrorNote error={error} /> : null}

      <div className="mt-4">
        <label className="label" htmlFor="raw">
          Paste an <code>/admin/keys</code> response <span className="normal-case text-faint">(optional)</span>
        </label>
        <textarea
          id="raw"
          className="textarea font-mono text-[12px]"
          placeholder='[{"key_id":"…","curve":"frost_secp256k1","ethereum_address":"0x…"}]'
          value={raw}
          onChange={(e) => setRaw(e.target.value)}
        />
      </div>
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
