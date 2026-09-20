"use client";

import { useMemo } from "react";
import {
  Badge,
  Callout,
  EmptyState,
  ErrorNote,
  InfoTip,
  PageHeader,
  Section,
  SkeletonRows,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useQuery } from "@/lib/hooks";
import { useSession } from "@/providers/SessionProvider";
import { chainName, formatDateTime, relativeTime, shortAddress } from "@/lib/format";
import type { StaffOverview } from "@/lib/types";

/**
 * The staff overview.
 *
 * This is the only screen that reads across tenants, and it exists for one
 * reason: sponsorship is open, so the platform pays to deploy groups for people
 * nobody vetted. That is the right posture while the network is courting
 * developers — but "watch for abuse and react" is only a strategy if someone can
 * watch, and before this the first signal would have been the paymaster balance
 * dropping.
 *
 * So it is ordered by what it costs, not by what is newest: accounts are listed
 * heaviest sponsorship first, because the question this answers is "is anyone
 * taking advantage", not "who signed up".
 *
 * Read-only, deliberately. Staff can see who is using the platform and cannot
 * reach into an organization or act for anyone. Observing a tenant is already a
 * privilege worth keeping narrow.
 */
export default function StaffPage() {
  const { user } = useSession();
  const overview = useQuery(() => api.staffOverview(), []);
  const data = overview.data;

  const ceiling = data?.sponsorship.max_per_subject ?? 0;

  // Whoever is closest to their allowance is who staff should look at first.
  const atCeiling = useMemo(
    () => (data?.users ?? []).filter((u) => ceiling > 0 && u.sponsored_groups >= ceiling),
    [data?.users, ceiling],
  );

  if (!user?.is_staff) {
    return (
      <>
        <PageHeader eyebrow="Staff" title="Platform overview" />
        <Callout tone="warn" title="This page is restricted to platform staff">
          Staff is granted by sign-in subject through STAFF_SUBJECTS, so it is tied to the
          credential you signed in with rather than to your person. Signing in by the other route
          gives you a different subject and no staff rights.
        </Callout>
      </>
    );
  }

  return (
    <>
      <PageHeader
        eyebrow="Staff"
        title="Platform overview"
        actions={
          <button type="button" className="btn-ghost" onClick={() => overview.refresh()}>
            Refresh
          </button>
        }
      />

      {overview.error ? <ErrorNote error={overview.error} onRetry={overview.refresh} /> : null}

      {overview.loading && !data ? (
        <SkeletonRows rows={6} />
      ) : data ? (
        <div className="space-y-6">
          {/* Sponsorship first: it is the thing that spends money. */}
          <Section
            title="Sponsorship"
            info="What the deploy route is enforcing right now, read from the server's own configuration rather than inferred."
          >
            <dl className="grid gap-x-8 gap-y-5 sm:grid-cols-2 lg:grid-cols-4">
              <Stat
                label="Status"
                value={
                  !data.sponsorship.enabled
                    ? "Off"
                    : data.sponsorship.invitation_only
                      ? `Invitation only · ${data.sponsorship.invited_count}`
                      : "Open to anyone signed in"
                }
                tone={data.sponsorship.enabled && !data.sponsorship.invitation_only ? "warn" : "neutral"}
              />
              <Stat
                label="Ceiling per person"
                value={ceiling > 0 ? `${ceiling} groups` : "None"}
                tone={ceiling > 0 ? "neutral" : "warn"}
              />
              <Stat label="Groups sponsored" value={data.totals.sponsored_groups} />
              <Stat label="At their ceiling" value={atCeiling.length} tone={atCeiling.length > 0 ? "warn" : "neutral"} />
            </dl>

            {data.sponsorship.enabled && !data.sponsorship.invitation_only ? (
              <Callout
                tone="accent"
                title={
                  <>
                    Anyone who signs in can have a group deployed at our expense
                    <InfoTip>
                      Intended while the network is courting developers. The per-person ceiling
                      bounds one account; the shared limit is the paymaster deposit, which is the
                      number to keep modest and alarmed. Set SPONSORED_SUBJECTS to make it
                      invitation-only.
                    </InfoTip>
                  </>
                }
              />
            ) : null}
          </Section>

          <Section title="Totals">
            <dl className="grid gap-x-8 gap-y-5 sm:grid-cols-3 lg:grid-cols-5">
              <Stat
                label="Accounts"
                value={data.totals.users}
                hint={`${data.totals.signet_users} Signet · ${data.totals.wallet_users} wallet`}
              />
              <Stat label="Organizations" value={data.totals.organizations} />
              <Stat label="Apps" value={data.totals.apps} />
              <Stat label="Deployed groups" value={data.totals.deployed_groups} />
              <Stat
                label="Last 24 hours"
                value={`${data.totals.new_users_24h} / ${data.totals.new_groups_24h}`}
                hint="accounts / groups"
              />
            </dl>
          </Section>

          <Section
            title="Accounts"
            info="Ordered by sponsored groups, so whoever is costing the most appears first."
          >
            {data.users.length === 0 ? (
              <EmptyState title="Nobody has signed in yet" />
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-[13px]">
                  <thead>
                    <tr className="border-b text-left text-[11px] font-semibold uppercase tracking-[0.08em] text-faint hairline">
                      <th className="pb-2 pr-4 font-semibold">Subject</th>
                      <th className="pb-2 pr-4 font-semibold">Route</th>
                      <th className="pb-2 pr-4 font-semibold">Orgs</th>
                      <th className="pb-2 pr-4 font-semibold">Apps</th>
                      <th className="pb-2 pr-4 font-semibold">Sponsored</th>
                      <th className="pb-2 font-semibold">Last seen</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.users.map((u) => (
                      <tr key={u.subject} className="border-b hairline">
                        <td className="py-2.5 pr-4">
                          <span className="mono text-fg">{shortSubject(u.subject)}</span>
                          {u.is_staff ? (
                            <span className="ml-2">
                              <Badge tone="accent">staff</Badge>
                            </span>
                          ) : null}
                          {u.display_name ? (
                            <span className="ml-2 text-muted">{u.display_name}</span>
                          ) : null}
                        </td>
                        <td className="py-2.5 pr-4 text-muted">
                          {u.subject_kind === "signet" ? "Signet" : "Wallet"}
                        </td>
                        <td className="py-2.5 pr-4 text-muted">{u.orgs}</td>
                        <td className="py-2.5 pr-4 text-muted">{u.apps}</td>
                        <td className="py-2.5 pr-4">
                          {ceiling > 0 && u.sponsored_groups >= ceiling ? (
                            <Badge tone="warn">
                              {u.sponsored_groups} / {ceiling}
                            </Badge>
                          ) : (
                            <span className="text-muted">
                              {u.sponsored_groups}
                              {ceiling > 0 ? ` / ${ceiling}` : ""}
                            </span>
                          )}
                        </td>
                        <td className="py-2.5 text-muted">
                          {u.last_seen_at ? relativeTime(u.last_seen_at) : "—"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Section>

          <Section title="Apps" info="Newest first, across every organization.">
            {data.apps.length === 0 ? (
              <EmptyState title="No apps yet" />
            ) : (
              <ul className="space-y-2.5">
                {data.apps.map((a, i) => (
                  <li
                    key={`${a.org_name}-${a.name}-${i}`}
                    className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 border-b pb-2.5 hairline"
                  >
                    <span className="min-w-0">
                      <span className="text-[13.5px] font-medium text-fg">{a.name}</span>
                      <span className="ml-2 text-[12.5px] text-muted">{a.org_name}</span>
                      {a.creator_subject ? (
                        <span className="mono ml-2 text-[12px] text-faint">
                          {shortSubject(a.creator_subject)}
                        </span>
                      ) : null}
                    </span>
                    <span className="flex flex-none items-center gap-2 text-[12.5px] text-muted">
                      {a.sponsored ? <Badge tone="accent">sponsored</Badge> : null}
                      <Badge tone={a.environment === "production" ? "error" : "neutral"}>
                        {a.environment}
                      </Badge>
                      {a.group_address ? (
                        <span className="mono">
                          {shortAddress(a.group_address)}
                          {a.threshold && a.node_count ? ` · ${a.threshold}-of-${a.node_count}` : ""}
                        </span>
                      ) : (
                        <span className="text-faint">no group</span>
                      )}
                      <span>{a.deployed_at ? formatDateTime(a.deployed_at) : "—"}</span>
                      <span className="text-faint">{chainName(a.chain_id)}</span>
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </Section>
        </div>
      ) : null}
    </>
  );
}

/** A Signet subject is 33 bytes of hex; showing all of it buries the row. */
function shortSubject(subject: string): string {
  const [kind, rest] = subject.split(":", 2);
  if (!rest) return subject;
  if (rest.length <= 20) return subject;
  return `${kind}:${rest.slice(0, 8)}…${rest.slice(-6)}`;
}

function Stat({
  label,
  value,
  hint,
  tone = "neutral",
}: {
  label: string;
  value: string | number;
  hint?: string;
  tone?: "neutral" | "warn";
}) {
  return (
    <div className="min-w-0">
      <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">{label}</dt>
      <dd
        className={`mt-1 text-[15.5px] font-semibold ${
          tone === "warn" ? "text-accent-600 dark:text-accent-400" : "text-fg"
        }`}
      >
        {value}
      </dd>
      {hint ? <dd className="mt-0.5 text-[12px] text-muted">{hint}</dd> : null}
    </div>
  );
}
