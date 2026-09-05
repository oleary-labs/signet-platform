"use client";

import { Callout, ErrorNote, PageHeader, Section, Skeleton, StatCard, StatusDot } from "@/components/ui";
import { NodeCard } from "@/components/NodeCard";
import { api } from "@/lib/api";
import { useQuery } from "@/lib/hooks";
import { chainName, formatDateTime, shortAddress } from "@/lib/format";

export default function StatusPage() {
  const status = useQuery(() => api.status(), []);
  const operators = useQuery(() => api.nodeOperators(), []);

  return (
    <div className="container-page py-14">
      <PageHeader
        eyebrow="Network status"
        title="What the network is doing right now"
        actions={
          <button
            type="button"
            className="btn-ghost"
            onClick={() => {
              status.refresh();
              operators.refresh();
            }}
          >
            Refresh
          </button>
        }
      />

      {status.error ? <ErrorNote error={status.error} onRetry={status.refresh} /> : null}

      {status.loading ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-28 rounded-2xl" />
          ))}
        </div>
      ) : status.data ? (
        <>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatCard
              label="Chain"
              value={chainName(status.data.chain_id)}
              tone={status.data.chain_reachable ? "success" : "error"}
              hint={
                status.data.chain_reachable
                  ? `Factory ${shortAddress(status.data.factory_address)}`
                  : "The RPC endpoint is not reachable from the platform."
              }
            />
            <StatCard
              label="Registered nodes"
              value={status.data.chain_reachable ? status.data.registered_nodes : "—"}
              hint="Nodes registered with the factory contract."
            />
            <StatCard
              label="Signing groups"
              value={status.data.chain_reachable ? status.data.groups : "—"}
              hint="Groups deployed by the factory, across every app."
            />
            <StatCard
              label="Operators online"
              value={`${status.data.operators_online} / ${status.data.operators_listed}`}
              tone={
                status.data.operators_online === status.data.operators_listed ? "success" : "accent"
              }
              hint="Listed operators responding to health probes."
            />
          </div>

          {status.data.chain_error ? (
            <Callout tone="warn" title="Chain reads are degraded">
              <p>
                The marketplace and console still work — group state simply shows the last value
                the platform was able to read. The error was:
              </p>
              <code className="mt-2 block break-words">{status.data.chain_error}</code>
            </Callout>
          ) : null}

          <p className="mt-4 text-[12.5px] text-faint">
            Checked {formatDateTime(status.data.checked_at)}.
          </p>
        </>
      ) : null}

      <div className="mt-10">
        <Section
          title="Operators"
        >
          {operators.loading ? (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-[236px] rounded-2xl" />
              ))}
            </div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {(operators.data ?? []).map((o) => (
                <NodeCard key={o.address} operator={o} href={`/marketplace/${o.address}`} />
              ))}
            </div>
          )}
        </Section>
      </div>

      <div className="mt-8 flex items-center gap-2 text-[13px] text-muted">
        <StatusDot tone="success" pulse />
        Probes run every couple of minutes. A node that stops responding shows as unreachable
        within one interval.
      </div>
    </div>
  );
}
