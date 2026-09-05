"use client";

import Link from "next/link";
import { use } from "react";
import {
  Badge,
  Callout,
  CopyValue,
  ErrorNote,
  PageHeader,
  Section,
  Skeleton,
  StatCard,
  StatusDot,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useQuery } from "@/lib/hooks";
import { formatDate, formatDateTime } from "@/lib/format";

export default function OperatorPage({ params }: { params: Promise<{ address: string }> }) {
  const { address } = use(params);
  const query = useQuery(() => api.nodeOperator(address), [address]);

  if (query.loading) {
    return (
      <div className="container-page space-y-4 py-14">
        <Skeleton className="h-10 w-72" />
        <Skeleton className="h-40 w-full rounded-2xl" />
      </div>
    );
  }
  if (query.error || !query.data) {
    return (
      <div className="container-page py-14">
        <ErrorNote error={query.error ?? new Error("Operator not found")} onRetry={query.refresh} />
        <Link href="/marketplace" className="btn-ghost mt-5">
          Back to the marketplace
        </Link>
      </div>
    );
  }

  const o = query.data;
  const health = o.health;

  return (
    <div className="container-page py-14">
      <Link href="/marketplace" className="mb-6 inline-block text-[13px] text-muted hover:text-fg">
        ← All operators
      </Link>

      <PageHeader
        eyebrow={o.verified ? "Reviewed operator" : "Listed operator"}
        title={o.name}
        actions={
          o.website_url ? (
            <a href={o.website_url} target="_blank" rel="noreferrer" className="btn-ghost">
              Visit website
            </a>
          ) : null
        }
      />

      <div className="mb-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          label="Status"
          tone={health?.online ? "success" : health ? "error" : undefined}
          value={
            <span className="inline-flex items-center gap-2">
              {health ? (
                <>
                  <StatusDot tone={health.online ? "success" : "error"} pulse={health.online} />
                  {health.online ? "Online" : "Unreachable"}
                </>
              ) : (
                <span className="text-muted">Unprobed</span>
              )}
            </span>
          }
          hint={
            health?.observed_at
              ? `Last checked ${formatDateTime(health.observed_at)}`
              : "The platform has not been able to reach this node's API yet."
          }
        />
        <StatCard
          label="Uptime · 24h"
          value={
            health?.uptime_pct_24h !== null && health?.uptime_pct_24h !== undefined
              ? `${health.uptime_pct_24h.toFixed(1)}%`
              : "—"
          }
          hint="Share of the platform's health probes that succeeded."
        />
        <StatCard
          label="Signing latency"
          value={health?.latency_ms !== null && health?.latency_ms !== undefined ? `${health.latency_ms} ms` : "—"}
          hint="Round-trip to the node's health endpoint from the platform, not end-to-end signing time."
        />
        <StatCard
          label="Groups served"
          value={o.group_count}
          hint="Signing groups this node is an active member of."
        />
      </div>

      <div className="grid items-start gap-5 lg:grid-cols-3">
        <Section title="Operator" className="lg:col-span-2">
          <dl className="grid gap-x-8 gap-y-4 sm:grid-cols-2">
            <Field label="Node address">
              <CopyValue value={o.address} label="Node address" />
            </Field>
            <Field label="Registry operator key">
              {o.operator_address ? (
                <CopyValue value={o.operator_address} label="Operator key" />
              ) : (
                <span className="text-muted">The node is its own operator</span>
              )}
            </Field>
            <Field label="Category">{o.category}</Field>
            <Field label="Region">{o.region || "—"}</Field>
            <Field label="Jurisdiction">{o.jurisdiction || "—"}</Field>
            <Field label="Registered">{formatDate(o.registered_at)}</Field>
            <Field label="Invitations">
              {o.is_open === true ? (
                <Badge tone="success" dot>
                  Accepts automatically
                </Badge>
              ) : o.is_open === false ? (
                <Badge tone="warn" dot>
                  Requires acceptance
                </Badge>
              ) : (
                <Badge tone="neutral">Not read from chain</Badge>
              )}
            </Field>
            <Field label="API endpoint">
              {o.api_url ? <CopyValue value={o.api_url} /> : <span className="text-muted">Not published</span>}
            </Field>
          </dl>
        </Section>

        <div className="space-y-5">
          <Section title="What this operator can and cannot do">
            <ul className="space-y-3 text-[13.5px] leading-relaxed text-muted">
              <li className="flex gap-2.5">
                <span className="mt-1.5 h-1.5 w-1.5 flex-none rounded-full bg-success-500" />
                Holds one share of each key in the groups it belongs to — never a whole key.
              </li>
              <li className="flex gap-2.5">
                <span className="mt-1.5 h-1.5 w-1.5 flex-none rounded-full bg-success-500" />
                Verifies your users&rsquo; credentials independently before contributing a share.
              </li>
              <li className="flex gap-2.5">
                <span className="mt-1.5 h-1.5 w-1.5 flex-none rounded-full bg-error-500" />
                Cannot produce a signature alone, or block one if your threshold is still met.
              </li>
              <li className="flex gap-2.5">
                <span className="mt-1.5 h-1.5 w-1.5 flex-none rounded-full bg-error-500" />
                Cannot add itself to your group, or stop you removing it.
              </li>
            </ul>
          </Section>

          {health?.last_error ? (
            <Callout tone="error" title="Last probe error">
              <code className="break-words">{health.last_error}</code>
            </Callout>
          ) : null}

          {!o.verified ? (
            <Callout tone="warn" title="Not yet reviewed">
              This operator is registered on-chain but has not been reviewed by the Signet team.
              Its self-reported details have not been checked.
            </Callout>
          ) : null}

          <Link href="/apps/new" className="btn-accent w-full">
            Build a group with this operator
          </Link>
        </div>
      </div>
    </div>
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
