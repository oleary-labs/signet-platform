"use client";

import { use, useState } from "react";
import {
  Badge,
  Callout,
  EmptyState,
  ErrorNote,
  Modal,
  PageHeader,
  Section,
  SkeletonRows,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useAction, useQuery } from "@/lib/hooks";
import { useToast } from "@/providers/ToastProvider";
import type { EnforcedBy, Policy, PolicyKind } from "@/lib/types";

const KINDS: { id: PolicyKind; label: string; enforcedBy: EnforcedBy; description: string }[] = [
  {
    id: "key_scope",
    label: "Key scope",
    enforcedBy: "node",
    description:
      "Bind a key to one chain, contract, and message type. Every operator re-checks this before contributing a share, so it holds even if the node handling the request is hostile.",
  },
  {
    id: "allowlist",
    label: "Destination allowlist",
    enforcedBy: "smart_account",
    description:
      "Only these addresses may receive value. Enforced by the account on-chain, atomically, at execution.",
  },
  {
    id: "denylist",
    label: "Destination denylist",
    enforcedBy: "smart_account",
    description: "Block specific addresses. Enforced on-chain alongside the allowlist.",
  },
  {
    id: "spend_limit",
    label: "Spend limit",
    enforcedBy: "advisory",
    description:
      "A cap per period. Recorded here and enforced by your own server — the protocol has no per-value policy engine yet.",
  },
  {
    id: "rate_limit",
    label: "Rate limit",
    enforcedBy: "advisory",
    description:
      "A ceiling on operations per period. Recorded here and enforced by your own server.",
  },
];

const ENFORCEMENT_COPY: Record<EnforcedBy, { label: string; tone: "success" | "accent" | "warn" }> = {
  node: { label: "Enforced by every operator", tone: "success" },
  smart_account: { label: "Enforced on-chain", tone: "accent" },
  advisory: { label: "Enforced by your server", tone: "warn" },
};

export default function PoliciesPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);
  const toast = useToast();
  const policies = useQuery(() => api.policies(appId), [appId]);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Policy | null>(null);

  const [remove] = useAction(async (id: string) => {
    await api.deletePolicy(appId, id);
    await policies.refresh();
    toast.success("Policy deleted");
  });

  return (
    <>
      <PageHeader
        title="Policies"
        info="Watch the badge on each rule. A key scope is checked by every operator; a smart-account rule by the chain; anything marked advisory is enforced by your own server, not the network."
        actions={
          <button
            type="button"
            className="btn-accent"
            onClick={() => {
              setEditing(null);
              setOpen(true);
            }}
          >
            New policy
          </button>
        }
      /><Section>
        {policies.error ? <ErrorNote error={policies.error} onRetry={policies.refresh} /> : null}

        {policies.loading ? (
          <SkeletonRows rows={3} />
        ) : (policies.data ?? []).length === 0 ? (
          <EmptyState
            title="No policies yet"
            action={
              <button type="button" className="btn-accent" onClick={() => setOpen(true)}>
                Create a policy
              </button>
            }
          />
        ) : (
          <ul className="space-y-2.5">
            {policies.data!.map((p) => {
              const enforcement = ENFORCEMENT_COPY[p.enforced_by];
              return (
                <li key={p.id} className="rounded-xl border px-4 py-3.5 hairline">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="text-[13.5px] font-medium text-fg">{p.name}</p>
                        <Badge tone={enforcement.tone} dot>
                          {enforcement.label}
                        </Badge>
                        {!p.enabled ? <Badge tone="neutral">disabled</Badge> : null}
                      </div>
                      <p className="mt-1 text-[12.5px] text-muted">
                        {KINDS.find((k) => k.id === p.kind)?.label ?? p.kind}
                      </p>
                    </div>
                    <div className="flex flex-none gap-2">
                      <button
                        type="button"
                        className="btn-ghost btn-sm"
                        onClick={() => {
                          setEditing(p);
                          setOpen(true);
                        }}
                      >
                        Edit
                      </button>
                      <button type="button" className="btn-danger btn-sm" onClick={() => remove(p.id)}>
                        Delete
                      </button>
                    </div>
                  </div>
                  {Object.keys(p.config).length > 0 ? (
                    <pre className="mt-3 overflow-x-auto rounded-lg bg-fg/[0.04] px-3 py-2 font-mono text-[11.5px] text-muted">
                      {JSON.stringify(p.config, null, 2)}
                    </pre>
                  ) : null}
                </li>
              );
            })}
          </ul>
        )}
      </Section>

      <PolicyModal
        open={open}
        policy={editing}
        onClose={() => setOpen(false)}
        appId={appId}
        onDone={() => {
          setOpen(false);
          policies.refresh();
        }}
      />
    </>
  );
}

function PolicyModal({
  open,
  policy,
  onClose,
  appId,
  onDone,
}: {
  open: boolean;
  policy: Policy | null;
  onClose: () => void;
  appId: string;
  onDone: () => void;
}) {
  const toast = useToast();
  const [name, setName] = useState(policy?.name ?? "");
  const [kind, setKind] = useState<PolicyKind>(policy?.kind ?? "key_scope");
  const [config, setConfig] = useState(JSON.stringify(policy?.config ?? {}, null, 2));

  const selected = KINDS.find((k) => k.id === kind)!;

  const [save, { pending, error }] = useAction(async () => {
    let parsed: unknown;
    try {
      parsed = JSON.parse(config || "{}");
    } catch {
      throw new Error("The configuration is not valid JSON.");
    }
    await api.savePolicy(appId, { name: name.trim(), kind, config: parsed, enabled: true });
    toast.success("Policy saved");
    onDone();
  });

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={policy ? "Edit policy" : "New policy"}
      wide
      footer={
        <>
          <button type="button" className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            type="button"
            className="btn-accent"
            disabled={pending || !name.trim()}
            onClick={() => save()}
          >
            {pending ? "Saving…" : "Save policy"}
          </button>
        </>
      }
    >
      {error ? <ErrorNote error={error} /> : null}

      <div className="mt-4 space-y-5">
        <div>
          <label className="label" htmlFor="policy-name">
            Name
          </label>
          <input
            id="policy-name"
            className="input"
            placeholder="USDC payments on Base"
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={policy !== null}
          />
        </div>

        <div>
          <span className="label">Type</span>
          <div className="space-y-2">
            {KINDS.map((k) => {
              const enforcement = ENFORCEMENT_COPY[k.enforcedBy];
              return (
                <button
                  key={k.id}
                  type="button"
                  onClick={() => setKind(k.id)}
                  className={`w-full rounded-xl border px-3.5 py-3 text-left transition ${
                    kind === k.id ? "border-accent-500 bg-accent-500/[0.08]" : "hover:bg-fg/[0.03] hairline"
                  }`}
                >
                  <span className="flex flex-wrap items-center gap-2">
                    <span className="text-[13.5px] font-medium text-fg">{k.label}</span>
                    <Badge tone={enforcement.tone}>{enforcement.label}</Badge>
                  </span>
                  <span className="mt-1 block text-[12px] leading-snug text-muted">
                    {k.description}
                  </span>
                </button>
              );
            })}
          </div>
        </div>

        <div>
          <label className="label" htmlFor="policy-config">
            Configuration
          </label>
          <textarea
            id="policy-config"
            className="textarea font-mono text-[12px]"
            value={config}
            onChange={(e) => setConfig(e.target.value)}
            placeholder={placeholderFor(kind)}
          />
        </div>
      </div>
    </Modal>
  );
}

function placeholderFor(kind: PolicyKind): string {
  switch (kind) {
    case "key_scope":
      return JSON.stringify(
        { chain_id: 8453, contract: "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", primary_type: "TransferWithAuthorization" },
        null,
        2,
      );
    case "allowlist":
    case "denylist":
      return JSON.stringify({ addresses: ["0x…"] }, null, 2);
    case "spend_limit":
      return JSON.stringify({ asset: "USDC", amount: "100.00", period: "day" }, null, 2);
    case "rate_limit":
      return JSON.stringify({ operations: 100, period: "hour" }, null, 2);
  }
}
