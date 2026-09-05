"use client";

import Link from "next/link";
import { useMemo } from "react";
import { Badge, EmptyState, ErrorNote, PageHeader, Skeleton, StatusDot } from "@/components/ui";
import { api } from "@/lib/api";
import { useQuery } from "@/lib/hooks";
import { useSession } from "@/providers/SessionProvider";
import { formatDate, relativeTime } from "@/lib/format";
import type { App } from "@/lib/types";

const ENV_TONE = { production: "error", staging: "warn", development: "neutral" } as const;

export default function AppsPage() {
  const { activeOrg } = useSession();
  const query = useQuery(
    () => (activeOrg ? api.apps(activeOrg.id) : Promise.resolve([])),
    [activeOrg?.id],
  );

  const apps = useMemo(() => query.data ?? [], [query.data]);

  return (
    <>
      <PageHeader
        title="Apps"
        actions={
          <Link href="/apps/new" className="btn-accent">
            New app
          </Link>
        }
      />

      {query.error ? <ErrorNote error={query.error} onRetry={query.refresh} /> : null}

      {query.loading ? (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-44 rounded-2xl" />
          ))}
        </div>
      ) : apps.length === 0 ? (
        <EmptyState
          title="No apps yet"
          action={
            <Link href="/apps/new" className="btn-accent">
              Create your first app
            </Link>
          }
        />
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {apps.map((app, i) => (
            <AppCard key={app.id} app={app} index={i} />
          ))}
        </div>
      )}
    </>
  );
}

function AppCard({ app, index }: { app: App; index: number }) {
  const operational = app.is_operational;
  return (
    <Link
      href={`/apps/${app.id}`}
      className={`fx fx-d${(index % 4) + 1} card block p-5 transition hover:shadow-pop`}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="truncate text-[15.5px] font-semibold text-fg">{app.name}</h2>
          <p className="mt-0.5 truncate text-[12.5px] text-muted">
            Updated {relativeTime(app.updated_at)}
          </p>
        </div>
        <Badge tone={ENV_TONE[app.environment]}>{app.environment}</Badge>
      </div>

      {app.description ? (
        <p className="mt-3 line-clamp-2 text-[13.5px] leading-relaxed text-muted">
          {app.description}
        </p>
      ) : null}

      <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 border-t pt-3.5 text-[12.5px] text-muted hairline">
        {app.group_address ? (
          <>
            <span className="inline-flex items-center gap-1.5">
              <StatusDot
                tone={operational ? "success" : operational === false ? "error" : "neutral"}
                pulse={operational === true}
              />
              {operational === null
                ? "Not synced"
                : operational
                  ? "Operational"
                  : "Below threshold"}
            </span>
            {app.threshold && app.node_count ? (
              <span className="mono">
                {app.threshold}-of-{app.node_count}
              </span>
            ) : null}
          </>
        ) : (
          <span className="inline-flex items-center gap-1.5 text-accent-600 dark:text-accent-400">
            <StatusDot tone="warn" />
            Setup unfinished
          </span>
        )}
        {app.deployed_at ? <span>Live since {formatDate(app.deployed_at)}</span> : null}
      </div>
    </Link>
  );
}
