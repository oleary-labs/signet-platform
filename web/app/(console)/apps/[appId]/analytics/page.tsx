"use client";

import { use, useMemo, useState } from "react";
import { AreaChart, CHART_COLORS } from "@/components/Chart";
import { Callout, ErrorNote, PageHeader, Section, Skeleton, StatCard, Tabs } from "@/components/ui";
import { api } from "@/lib/api";
import { useQuery } from "@/lib/hooks";
import { formatNumber, formatPercent } from "@/lib/format";

const RANGES = [
  { id: "7", label: "7 days" },
  { id: "30", label: "30 days" },
  { id: "90", label: "90 days" },
];

export default function AnalyticsPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);
  const [range, setRange] = useState("30");

  const { from, to } = useMemo(() => {
    const end = new Date();
    const start = new Date(end);
    start.setDate(end.getDate() - (Number(range) - 1));
    const fmt = (d: Date) => d.toISOString().slice(0, 10);
    return { from: fmt(start), to: fmt(end) };
  }, [range]);

  const usage = useQuery(() => api.usage(appId, { from, to }), [appId, from, to]);
  const points = usage.data?.series ?? [];
  const labels = points.map((p) => p.day);

  const latency = points.filter((p) => p.p95_latency_ms !== null);

  return (
    <>
      <PageHeader
        title="Analytics"
        info="Active wallets is a distinct count over the whole window, not the sum of the daily bars — a returning user is one wallet. It is the unit billing uses."
        actions={
          <Tabs
            tabs={RANGES.map((r) => ({ id: r.id, label: r.label }))}
            active={range}
            onChange={setRange}
          />
        }
      />

      {usage.error ? <ErrorNote error={usage.error} onRetry={usage.refresh} /> : null}

      <div className="mb-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="Active wallets"
          value={formatNumber(usage.data?.monthly_active_wallets ?? 0)}
          hint="Distinct users across the whole window — not the sum of the daily counts, since a returning user is one wallet."
        />
        <StatCard label="Signatures" value={formatNumber(usage.data?.total_signs ?? 0)} />
        <StatCard label="Keys generated" value={formatNumber(usage.data?.total_keygens ?? 0)} />
        <StatCard
          label="Error rate"
          tone={
            (usage.data?.error_rate ?? 0) > 0.05
              ? "error"
              : (usage.data?.error_rate ?? 0) > 0.01
                ? "warn"
                : "success"
          }
          value={usage.data ? formatPercent(usage.data.error_rate, 2) : "—"}
          hint="Share of operations that failed, across auth, keygen, and signing."
        />
      </div>

      <div className="space-y-5">
        <Section
          title="Volume"
        >
          {usage.loading ? (
            <Skeleton className="h-[240px] rounded-xl" />
          ) : (
            <AreaChart
              labels={labels}
              height={260}
              series={[
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
                {
                  key: "keygens",
                  label: "Key generations",
                  color: CHART_COLORS.success,
                  values: points.map((p) => p.keygen_count),
                },
              ]}
            />
          )}
        </Section>

        <div className="grid gap-5 lg:grid-cols-2">
          <Section
            title="Latency"
          >
            {usage.loading ? (
              <Skeleton className="h-[200px] rounded-xl" />
            ) : latency.length === 0 ? (
              <p className="py-10 text-center text-[13.5px] text-muted">
                No latency has been reported for this range.
              </p>
            ) : (
              <AreaChart
                labels={latency.map((p) => p.day)}
                height={200}
                valueFormat={(v) => `${v}ms`}
                series={[
                  {
                    key: "p50",
                    label: "p50",
                    color: CHART_COLORS.success,
                    values: latency.map((p) => p.p50_latency_ms ?? 0),
                  },
                  {
                    key: "p95",
                    label: "p95",
                    color: CHART_COLORS.accent,
                    values: latency.map((p) => p.p95_latency_ms ?? 0),
                  },
                ]}
              />
            )}
          </Section>

          <Section
            title="Failures"
          >
            {usage.loading ? (
              <Skeleton className="h-[200px] rounded-xl" />
            ) : (
              <AreaChart
                labels={labels}
                height={200}
                series={[
                  {
                    key: "errors",
                    label: "Failed operations",
                    color: CHART_COLORS.error,
                    values: points.map((p) => p.error_count),
                  },
                ]}
              />
            )}
          </Section>
        </div>

      </div>
    </>
  );
}
