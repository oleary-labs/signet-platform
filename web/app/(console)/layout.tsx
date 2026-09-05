"use client";

import { useRouter, usePathname, useParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { Sidebar } from "@/components/console/Sidebar";
import { UserMenu } from "@/components/console/UserMenu";
import { Logo } from "@/components/Logo";
import { Skeleton } from "@/components/ui";
import { api } from "@/lib/api";
import { useQuery } from "@/lib/hooks";
import { useSession } from "@/providers/SessionProvider";

/**
 * The console shell.
 *
 * It gates on the session, resolves the app in the URL, and renders the
 * sidebar. The app list is fetched once here rather than per page, so the
 * switcher is populated on every screen without each one re-requesting it.
 */
export default function ConsoleLayout({ children }: { children: React.ReactNode }) {
  const { status, organizations, activeOrg, setActiveOrg } = useSession();
  const router = useRouter();
  const pathname = usePathname();
  const params = useParams<{ appId?: string }>();
  const [mobileNav, setMobileNav] = useState(false);

  useEffect(() => {
    if (status === "signed-out") {
      router.replace(`/login?next=${encodeURIComponent(pathname)}`);
    }
  }, [status, router, pathname]);

  // A signed-in developer with no organization has nothing to look at, so send
  // them to create one rather than showing an empty console.
  useEffect(() => {
    if (status === "signed-in" && organizations.length === 0 && pathname !== "/onboarding") {
      router.replace("/onboarding");
    }
  }, [status, organizations.length, pathname, router]);

  const appsQuery = useQuery(
    () => (status === "signed-in" ? api.myApps() : Promise.resolve([])),
    [status],
  );

  const apps = useMemo(() => appsQuery.data ?? [], [appsQuery.data]);
  const activeApp = useMemo(
    () => apps.find((a) => a.id === params.appId) ?? null,
    [apps, params.appId],
  );

  useEffect(() => setMobileNav(false), [pathname]);

  if (status === "loading") {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Logo size={34} className="anim-float text-faint" />
      </div>
    );
  }
  if (status === "signed-out") return null;

  return (
    <div className="flex min-h-screen">
      <div className="hidden lg:block">
        <Sidebar
          apps={apps}
          activeApp={activeApp}
          organizations={organizations}
          activeOrg={activeOrg}
          onSelectOrg={setActiveOrg}
        />
      </div>

      {mobileNav ? (
        <div className="fixed inset-0 z-50 flex lg:hidden">
          <div className="absolute inset-0 bg-primary-950/50" onClick={() => setMobileNav(false)} />
          <div className="relative">
            <Sidebar
              apps={apps}
              activeApp={activeApp}
              organizations={organizations}
              activeOrg={activeOrg}
              onSelectOrg={setActiveOrg}
              onNavigate={() => setMobileNav(false)}
            />
          </div>
        </div>
      ) : null}

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-20 flex h-[var(--header-h)] flex-none items-center justify-between gap-3 border-b bg-bg/85 px-4 backdrop-blur sm:px-6 hairline">
          <div className="flex min-w-0 items-center gap-3">
            <button
              type="button"
              onClick={() => setMobileNav(true)}
              className="btn-ghost btn-sm lg:hidden"
              aria-label="Open navigation"
            >
              <svg width="15" height="15" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                <path d="M2 4h12M2 8h12M2 12h12" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
              </svg>
            </button>
            {activeApp ? (
              <div className="min-w-0">
                <p className="truncate text-[13.5px] font-semibold text-fg">{activeApp.name}</p>
                <p className="truncate text-[11.5px] text-faint">
                  {activeApp.group_address
                    ? "Signing group attached"
                    : "No signing group yet — finish setup"}
                </p>
              </div>
            ) : null}
          </div>
          <UserMenu />
        </header>

        <main className="min-w-0 flex-1 px-4 py-7 sm:px-6 lg:px-8">
          {appsQuery.loading && !appsQuery.data ? (
            <div className="mx-auto max-w-page space-y-4">
              <Skeleton className="h-9 w-64" />
              <Skeleton className="h-40 w-full rounded-2xl" />
            </div>
          ) : (
            <div className="mx-auto max-w-page">{children}</div>
          )}
        </main>
      </div>
    </div>
  );
}
