"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import type { App, Organization } from "@/lib/types";
import { Badge } from "@/components/ui";

const ENV_TONE = {
  production: "error",
  development: "neutral",
} as const;

// Three letters, so the badge never crowds out the app name — which is the
// thing the switcher exists to show.
const ENV_SHORT = {
  production: "prod",
  development: "dev",
} as const;

/**
 * The app and organization switcher.
 *
 * The environment is shown on every row and in the trigger, because the single
 * most costly mistake a developer can make in a console like this is editing
 * production while believing they are in development.
 */
export function AppSwitcher({
  apps,
  activeApp,
  organizations,
  activeOrg,
  onSelectOrg,
}: {
  apps: App[];
  activeApp: App | null;
  organizations: Organization[];
  activeOrg: Organization | null;
  onSelectOrg: (orgId: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const router = useRouter();

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", onClick);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onClick);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const orgApps = apps.filter((a) => !activeOrg || a.org_id === activeOrg.id);

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex w-full items-center gap-2.5 rounded-xl border px-3 py-2.5 text-left transition hover:bg-fg/[0.04] hairline"
      >
        <span className="flex h-7 w-7 flex-none items-center justify-center rounded-lg bg-gradient-to-br from-primary-700 to-primary-500 text-[11px] font-bold text-white">
          {(activeApp?.name ?? activeOrg?.name ?? "?").slice(0, 2).toUpperCase()}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13.5px] font-semibold text-fg">
            {activeApp?.name ?? activeOrg?.name ?? "Select an app"}
          </span>
          <span className="block truncate text-[11.5px] text-muted">
            {activeApp ? activeOrg?.name : `${orgApps.length} app${orgApps.length === 1 ? "" : "s"}`}
          </span>
        </span>
        {activeApp ? (
          <Badge tone={ENV_TONE[activeApp.environment]}>{ENV_SHORT[activeApp.environment]}</Badge>
        ) : null}
        <svg width="12" height="12" viewBox="0 0 12 12" fill="none" className="flex-none text-faint" aria-hidden="true">
          <path d="m3 4.5 3 3 3-3" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>

      {open ? (
        <div className="absolute left-0 right-0 top-full z-30 mt-1.5 max-h-[70vh] overflow-y-auto rounded-xl border bg-surface p-1.5 shadow-pop hairline">
          {organizations.length > 1 ? (
            <>
              <p className="px-2.5 pb-1 pt-2 text-[10.5px] font-semibold uppercase tracking-[0.12em] text-faint">
                Organization
              </p>
              {organizations.map((o) => (
                <button
                  key={o.id}
                  type="button"
                  onClick={() => {
                    onSelectOrg(o.id);
                    setOpen(false);
                    router.push("/apps");
                  }}
                  className={`flex w-full items-center justify-between gap-2 rounded-lg px-2.5 py-2 text-left text-[13px] transition hover:bg-fg/[0.05] ${
                    activeOrg?.id === o.id ? "text-fg" : "text-muted"
                  }`}
                >
                  <span className="truncate">{o.name}</span>
                  {activeOrg?.id === o.id ? <span className="text-accent-500">✓</span> : null}
                </button>
              ))}
              <div className="my-1.5 border-t hairline" />
            </>
          ) : null}

          <p className="px-2.5 pb-1 pt-1 text-[10.5px] font-semibold uppercase tracking-[0.12em] text-faint">
            Apps
          </p>
          {orgApps.length === 0 ? (
            <p className="px-2.5 py-2 text-[13px] text-muted">No apps in this organization yet.</p>
          ) : (
            orgApps.map((a) => (
              <Link
                key={a.id}
                href={`/apps/${a.id}`}
                onClick={() => setOpen(false)}
                className={`flex items-center justify-between gap-2 rounded-lg px-2.5 py-2 text-[13px] transition hover:bg-fg/[0.05] ${
                  activeApp?.id === a.id ? "text-fg" : "text-muted"
                }`}
              >
                <span className="truncate">{a.name}</span>
                <Badge tone={ENV_TONE[a.environment]}>{ENV_SHORT[a.environment]}</Badge>
              </Link>
            ))
          )}

          <div className="my-1.5 border-t hairline" />
          <Link
            href="/apps/new"
            onClick={() => setOpen(false)}
            className="flex items-center gap-2 rounded-lg px-2.5 py-2 text-[13px] font-medium text-accent-600 transition hover:bg-accent-500/10 dark:text-accent-400"
          >
            + New app
          </Link>
        </div>
      ) : null}
    </div>
  );
}
