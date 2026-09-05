"use client";

import Link from "next/link";
import { use, useMemo } from "react";
import { useSearchParams } from "next/navigation";
import { AreaChart, CHART_COLORS } from "@/components/Chart";
import {
  Badge,
  Callout,
  CopyValue,
  ErrorNote,
  InfoTip,
  PageHeader,
  Section,
  Skeleton,
  StatCard,
  StatusDot,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useQuery } from "@/lib/hooks";
import { chainName, formatNumber, formatPercent, relativeTime, shortAddress } from "@/lib/format";

export default function AppOverviewPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);
  const search = useSearchParams();
  const justCreated = search.get("created") === "1";

  const app = useQuery(() => api.app(appId), [appId]);
  const group = useQuery(() => api.group(appId), [appId]);
  const usage = useQuery(() => api.usage(appId), [appId]);
  const keys = useQuery(() => api.keys(appId, { limit: "1" }), [appId]);

  const series = useMemo(() => {
    const points = usage.data?.series ?? [];
    return {
      labels: points.map((p) => p.day),
      data: [
        {
          key: "wallets",
          label: "Active wallets",
          color: CHART_COLORS.accent,
          values: points.map((p) => p.active_wallets),
        },
        {
          key: "signs",
          label: "Signatures",
          color: CHART_COLORS.primary,
          values: points.map((p) => p.sign_count),
        },
      ],
    };
  }, [usage.data]);

  if (app.error) return <ErrorNote error={app.error} onRetry={app.refresh} />;
  if (!app.data) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-9 w-64" />
        <Skeleton className="h-32 w-full rounded-2xl" />
      </div>
    );
  }

  const a = app.data;
  const onchain = group.data?.onchain;
  const activeNodes = onchain?.active_nodes.length ?? a.node_count ?? 0;
  const threshold = onchain?.threshold ?? a.threshold ?? 0;

  return (
    <>
      <PageHeader
        eyebrow={a.environment}
        title={a.name}
        actions={
          <>
            <Link href={`/apps/${appId}/group`} className="btn-ghost">
              Signing group
            </Link>
            <Link href={`/apps/${appId}/settings`} className="btn-ghost">
              Settings
            </Link>
          </>
        }
      />

      {justCreated ? (
        <div className="mb-6">
          <Callout
            tone="success"
            title={
              <>
                Signing group live
                <InfoTip>
                  Next: an application key so your backend can authenticate to the nodes, and the
                  origins your SDK will call from.
                </InfoTip>
              </>
            }
          >
            <div className="flex flex-wrap gap-2">
              <Link href={`/apps/${appId}/credentials`} className="btn-accent btn-sm">
                Create an application key
              </Link>
              <Link href={`/apps/${appId}/settings/domains`} className="btn-ghost btn-sm">
                Add a domain
              </Link>
            </div>
          </Callout>
        </div>
      ) : null}

      {!a.group_address ? (
        <div className="mb-6">
          <Callout
            tone="warn"
            title={
              <>
                No signing group attached
                <InfoTip>
                  The app exists on the platform but has no group on-chain, so it cannot sign
                  anything yet. Create one, or link a group you already control.
                </InfoTip>
              </>
            }
          >
            <div>
              <Link href={`/apps/${appId}/group`} className="btn-accent btn-sm">
                Finish setup
              </Link>
            </div>
          </Callout>
        </div>
      ) : null}

      <div className="mb-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="Group status"
          tone={a.is_operational ? "success" : a.is_operational === false ? "error" : undefined}
          value={
            <span className="inline-flex items-center gap-2">
              {a.group_address ? (
                <>
                  <StatusDot
                    tone={a.is_operational ? "success" : a.is_operational === false ? "error" : "neutral"}
                    pulse={a.is_operational === true}
                  />
                  {a.is_operational === null
                    ? "Unsynced"
                    : a.is_operational
                      ? "Operational"
                      : "Below threshold"}
                </>
              ) : (
                <span className="text-muted">Not deployed</span>
              )}
            </span>
          }
          hint={
            a.group_address
              ? `${threshold}-of-${activeNodes} on ${chainName(a.chain_id)}`
              : "Deploy a group to start signing."
          }
        />
        <StatCard
          label="Active wallets · 30d"
          value={formatNumber(usage.data?.monthly_active_wallets ?? 0)}
          hint="Unique users who generated a key or signed."
        />
        <StatCard
          label="Signatures · 30d"
          value={formatNumber(usage.data?.total_signs ?? 0)}
          hint={
            usage.data
              ? `${formatPercent(usage.data.error_rate)} of all operations failed`
              : undefined
          }
        />
        <StatCard
          label="Keys held"
          value={formatNumber(keys.data?.stats.total ?? 0)}
          hint={
            keys.data
              ? `${keys.data.stats.enabled} enabled · ${keys.data.stats.disabled} disabled`
              : "Sync from the group to populate."
          }
        />
      </div>

      <div className="grid items-start gap-5 lg:grid-cols-3">
        <Section
          title="Activity"
          className="lg:col-span-2"
          actions={
            <Link href={`/apps/${appId}/analytics`} className="btn-quiet btn-sm">
              Full analytics →
            </Link>
          }
        >
          {usage.loading ? (
            <Skeleton className="h-[220px] rounded-xl" />
          ) : (
            <AreaChart labels={series.labels} series={series.data} />
          )}
        </Section>

        <div className="space-y-5">
          <Section title="Integration">
            <dl className="space-y-4">
              <div className="min-w-0">
                <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                  App ID
                </dt>
                <dd className="mt-1">
                  <CopyValue value={a.id} label="App ID" />
                </dd>
              </div>
              {a.group_address ? (
                <div className="min-w-0">
                  <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                    Group address
                  </dt>
                  <dd className="mt-1">
                    <CopyValue
                      value={a.group_address}
                      display={shortAddress(a.group_address, 10, 8)}
                      label="Group address"
                    />
                  </dd>
                </div>
              ) : null}
              <div>
                <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                  Chain
                </dt>
                <dd className="mt-1 text-[13.5px] text-fg">{chainName(a.chain_id)}</dd>
              </div>
              <div>
                <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                  Last chain sync
                </dt>
                <dd className="mt-1 text-[13.5px] text-fg">
                  {a.synced_at ? relativeTime(a.synced_at) : "Never"}
                </dd>
              </div>
            </dl>

            <Link href={`/apps/${appId}/credentials`} className="btn-ghost mt-5 w-full">
              Keys & credentials
            </Link>
          </Section>

          <Section title="Operators">
            {group.loading ? (
              <Skeleton className="h-24 rounded-xl" />
            ) : (group.data?.nodes.length ?? 0) === 0 ? (
              <p className="text-[13.5px] text-muted">
                No membership cached yet. Open the group screen to sync it from the chain.
              </p>
            ) : (
              <ul className="space-y-2.5">
                {group.data!.nodes.slice(0, 5).map((n) => (
                  <li key={n.address} className="flex items-center justify-between gap-3">
                    <span className="flex min-w-0 items-center gap-2">
                      <StatusDot
                        tone={
                          n.status === "active"
                            ? n.operator?.health?.online
                              ? "success"
                              : "warn"
                            : n.status === "pending"
                              ? "warn"
                              : "error"
                        }
                      />
                      <span className="truncate text-[13px] text-fg">
                        {n.operator?.name ?? shortAddress(n.address)}
                      </span>
                    </span>
                    <Badge
                      tone={
                        n.status === "active" ? "success" : n.status === "pending" ? "warn" : "error"
                      }
                    >
                      {n.status}
                    </Badge>
                  </li>
                ))}
              </ul>
            )}
            <Link href={`/apps/${appId}/group`} className="btn-ghost mt-5 w-full">
              Manage the group
            </Link>
          </Section>
        </div>
      </div>
    </>
  );
}
