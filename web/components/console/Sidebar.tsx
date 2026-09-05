"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { App, Organization } from "@/lib/types";
import { Logo } from "@/components/Logo";
import { AppSwitcher } from "./AppSwitcher";
import { appNav, isActive, orgNav } from "./nav";

export function Sidebar({
  apps,
  activeApp,
  organizations,
  activeOrg,
  onSelectOrg,
  onNavigate,
}: {
  apps: App[];
  activeApp: App | null;
  organizations: Organization[];
  activeOrg: Organization | null;
  onSelectOrg: (orgId: string) => void;
  onNavigate?: () => void;
}) {
  const pathname = usePathname();
  const groups = [
    ...(activeApp ? appNav(activeApp.id) : []),
    ...(activeOrg ? orgNav(activeOrg.id) : []),
  ];

  return (
    <aside className="flex h-full w-[248px] flex-none flex-col border-r bg-surface hairline">
      <div className="flex h-[var(--header-h)] items-center gap-2.5 border-b px-4 hairline">
        <Link href="/" className="flex items-center gap-2.5 text-fg transition hover:opacity-80">
          <Logo size={24} />
          <span className="text-[15px] font-semibold tracking-tight">Signet</span>
        </Link>
      </div>

      <div className="p-3">
        <AppSwitcher
          apps={apps}
          activeApp={activeApp}
          organizations={organizations}
          activeOrg={activeOrg}
          onSelectOrg={onSelectOrg}
        />
      </div>

      <nav className="flex-1 overflow-y-auto px-3 pb-6">
        {!activeApp ? (
          <div className="space-y-1 pt-1">
            <Link
              href="/apps"
              onClick={onNavigate}
              className={`nav-link ${pathname === "/apps" ? "nav-link-active" : ""}`}
            >
              All apps
            </Link>
            <Link href="/apps/new" onClick={onNavigate} className="nav-link">
              New app
            </Link>
          </div>
        ) : null}

        {groups.map((group, gi) => (
          <div key={group.title ?? `g${gi}`}>
            {group.title ? <p className="nav-heading">{group.title}</p> : <div className="pt-1" />}
            <div className="space-y-0.5">
              {group.items.map((item) => (
                <Link
                  key={item.href}
                  href={item.href}
                  onClick={onNavigate}
                  className={`nav-link ${isActive(pathname, item) ? "nav-link-active" : ""}`}
                >
                  {item.label}
                </Link>
              ))}
            </div>
          </div>
        ))}
      </nav>

      <div className="border-t p-3 hairline">
        <Link href="/marketplace" onClick={onNavigate} className="nav-link">
          Operator marketplace
        </Link>
        <Link href="/docs" onClick={onNavigate} className="nav-link">
          Documentation
        </Link>
      </div>
    </aside>
  );
}
