"use client";

import { use, useState } from "react";
import {
  Badge,
  Callout,
  CopyValue,
  EmptyState,
  ErrorNote,
  PageHeader,
  Section,
  SkeletonRows,
  Toggle,
} from "@/components/ui";
import { SettingsForm } from "@/components/console/SettingsForm";
import { api } from "@/lib/api";
import { useAction, useQuery, useTicker } from "@/lib/hooks";
import { useToast } from "@/providers/ToastProvider";
import { countdown, curveLabel, formatDateTime, shortHash } from "@/lib/format";

/**
 * Session signers.
 *
 * The protocol calls these delegation tokens: a JWT signed by a parent key
 * that lets an agent use one scoped sub-key without the user's OAuth session.
 * The screen is careful about what revocation does, because a database row
 * cannot invalidate a signature — only disabling the sub-key on the nodes can.
 */
export default function SessionSignersPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);
  const toast = useToast();
  const [includeRevoked, setIncludeRevoked] = useState(false);
  useTicker(30_000);

  const delegations = useQuery(
    () => api.delegations(appId, includeRevoked),
    [appId, includeRevoked],
  );

  const [revoke] = useAction(async (id: string) => {
    const result = await api.revokeDelegation(appId, id);
    await delegations.refresh();
    toast.info("Marked revoked", result.note);
  });

  return (
    <>
      <PageHeader
        title="Session signers"
        info="Revoking here is a record, not enforcement. A delegation token is verified by the nodes against the parent key, so it keeps working until you disable the sub-key from Wallets."
      />

      <div className="space-y-5">
        <SettingsForm<"session_signers">
          appId={appId}
          section="session_signers"
          defaults={{
            enabled: true,
            default_ttl_seconds: 2_592_000,
            max_ttl_seconds: 7_776_000,
            require_scope: true,
          }}
        >
          {(value, set) => (
            <Section title="Policy">
              <div className="space-y-5">
                <Toggle
                  checked={value.enabled ?? true}
                  onChange={(v) => set({ enabled: v })}
                  label="Allow session signers"
                />
                <Toggle
                  checked={value.require_scope ?? true}
                  onChange={(v) => set({ require_scope: v })}
                  label="Require a scope on every delegated key"
                />

                <div className="grid gap-5 sm:grid-cols-2">
                  <div>
                    <label className="label" htmlFor="default-ttl">
                      Default lifetime (days)
                    </label>
                    <input
                      id="default-ttl"
                      type="number"
                      min={1}
                      className="input"
                      value={Math.round((value.default_ttl_seconds ?? 2_592_000) / 86_400)}
                      onChange={(e) => set({ default_ttl_seconds: Number(e.target.value) * 86_400 })}
                    />
                  </div>
                  <div>
                    <label className="label" htmlFor="max-ttl">
                      Maximum lifetime (days)
                    </label>
                    <input
                      id="max-ttl"
                      type="number"
                      min={1}
                      className="input"
                      value={Math.round((value.max_ttl_seconds ?? 7_776_000) / 86_400)}
                      onChange={(e) => set({ max_ttl_seconds: Number(e.target.value) * 86_400 })}
                    />
                  </div>
                </div>
              </div>
            </Section>
          )}
        </SettingsForm>

        <Section
          title="Issued signers"
          actions={
            <label className="flex cursor-pointer items-center gap-2 text-[13px] text-muted">
              <input
                type="checkbox"
                checked={includeRevoked}
                onChange={(e) => setIncludeRevoked(e.target.checked)}
                className="accent-[#e8873c]"
              />
              Show revoked
            </label>
          }
        >
          {delegations.error ? (
            <ErrorNote error={delegations.error} onRetry={delegations.refresh} />
          ) : null}

          {delegations.loading ? (
            <SkeletonRows rows={3} />
          ) : (delegations.data ?? []).length === 0 ? (
            <EmptyState
              title="No session signers issued"
            />
          ) : (
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>Label</th>
                    <th>Key</th>
                    <th>Scheme</th>
                    <th>Issued</th>
                    <th>Expires</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {delegations.data!.map((d) => {
                    const remaining = countdown(d.expires_at);
                    const expired = remaining === null;
                    return (
                      <tr key={d.id}>
                        <td className="text-[13.5px] text-fg">{d.label || "Untitled"}</td>
                        <td className="min-w-0">
                          <CopyValue value={d.key_id} display={shortHash(d.key_id, 18, 8)} />
                        </td>
                        <td className="whitespace-nowrap text-[13px] text-muted">
                          {curveLabel(d.curve)}
                        </td>
                        <td className="whitespace-nowrap text-[13px] text-muted">
                          {formatDateTime(d.issued_at)}
                        </td>
                        <td className="whitespace-nowrap">
                          {d.revoked_at ? (
                            <Badge tone="error">revoked</Badge>
                          ) : expired ? (
                            <Badge tone="neutral">expired</Badge>
                          ) : (
                            <span className="mono text-muted">{remaining}</span>
                          )}
                        </td>
                        <td className="text-right">
                          {!d.revoked_at && !expired ? (
                            <button
                              type="button"
                              className="btn-danger btn-sm"
                              onClick={() => revoke(d.id)}
                            >
                              Revoke
                            </button>
                          ) : null}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}

        </Section>
      </div>
    </>
  );
}
