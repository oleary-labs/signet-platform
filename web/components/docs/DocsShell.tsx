"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

export const DOCS_NAV: { title: string; items: { href: string; label: string }[] }[] = [
  {
    title: "Getting started",
    items: [
      { href: "/docs", label: "Overview" },
      { href: "/docs/quickstart", label: "Quickstart" },
      { href: "/docs/concepts", label: "Core concepts" },
    ],
  },
  {
    title: "Building",
    items: [
      { href: "/docs/auth", label: "Authentication" },
      { href: "/docs/keys", label: "Keys and signing" },
      { href: "/docs/scoped-keys", label: "Scoped sub-keys" },
      { href: "/docs/session-signers", label: "Session signers" },
    ],
  },
  {
    title: "Platform",
    items: [
      { href: "/docs/api", label: "Platform API" },
      { href: "/docs/webhooks", label: "Webhooks" },
      { href: "/docs/operators", label: "Running a node" },
    ],
  },
];

export function DocsShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  return (
    <div className="container-page grid gap-10 py-12 lg:grid-cols-[220px_minmax(0,1fr)]">
      <nav className="lg:sticky lg:top-[calc(var(--header-h)+24px)] lg:h-fit">
        {DOCS_NAV.map((group) => (
          <div key={group.title} className="mb-6">
            <p className="mb-2 text-[10.5px] font-semibold uppercase tracking-[0.12em] text-faint">
              {group.title}
            </p>
            <ul className="space-y-0.5">
              {group.items.map((item) => (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    className={`block rounded-lg px-2.5 py-1.5 text-[13.5px] transition ${
                      pathname === item.href
                        ? "bg-accent-500/[0.12] font-medium text-accent-700 dark:text-accent-300"
                        : "text-muted hover:bg-fg/[0.05] hover:text-fg"
                    }`}
                  >
                    {item.label}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </nav>

      <article className="docs-prose min-w-0 max-w-prose">{children}</article>
    </div>
  );
}

/** A code block with a title strip, matching the house style. */
export function Code({ title, children }: { title?: string; children: string }) {
  return (
    <div className="code-block not-prose my-5">
      {title ? (
        <div className="code-title">
          <span>{title}</span>
        </div>
      ) : null}
      <pre>{children}</pre>
    </div>
  );
}

/** A callout for the things that are easy to get wrong. */
export function Note({
  tone = "info",
  title,
  children,
}: {
  tone?: "info" | "warn";
  title?: string;
  children: React.ReactNode;
}) {
  return (
    <div
      className={`not-prose my-5 rounded-xl border px-4 py-3.5 ${
        tone === "warn"
          ? "border-accent-500/30 bg-accent-500/[0.07]"
          : "border-primary-500/25 bg-primary-500/[0.05]"
      }`}
    >
      {title ? <p className="text-[13px] font-semibold text-fg">{title}</p> : null}
      <div className={`text-[13.5px] leading-relaxed text-muted ${title ? "mt-1" : ""}`}>
        {children}
      </div>
    </div>
  );
}
