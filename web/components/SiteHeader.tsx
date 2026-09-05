"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { Wordmark } from "./Logo";
import { ThemeToggle } from "./ThemeToggle";
import { useSession } from "@/providers/SessionProvider";

const NAV = [
  { href: "/marketplace", label: "Operators" },
  { href: "/docs", label: "Docs" },
  { href: "/status", label: "Status" },
  { href: "/pricing", label: "Pricing" },
];

export function SiteHeader() {
  const pathname = usePathname();
  const { status } = useSession();
  const [scrolled, setScrolled] = useState(false);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 8);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  useEffect(() => setOpen(false), [pathname]);

  return (
    <header
      className={`fixed inset-x-0 top-0 z-40 transition-shadow ${
        scrolled ? "glass border-b hairline" : ""
      }`}
      style={{ height: "var(--header-h)" }}
    >
      <div className="container-page flex h-full items-center justify-between gap-6">
        <Link href="/" className="flex-none text-fg transition hover:opacity-80">
          <Wordmark />
        </Link>

        <nav className="hidden items-center gap-1 md:flex">
          {NAV.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className={`rounded-lg px-3 py-2 text-[13.5px] font-medium transition ${
                pathname.startsWith(item.href)
                  ? "text-fg"
                  : "text-muted hover:text-fg"
              }`}
            >
              {item.label}
            </Link>
          ))}
        </nav>

        <div className="flex flex-none items-center gap-2">
          <ThemeToggle />
          {status === "signed-in" ? (
            <Link href="/apps" className="btn-accent btn-sm">
              Console
            </Link>
          ) : (
            <>
              <Link href="/login" className="btn-quiet btn-sm hidden sm:inline-flex">
                Sign in
              </Link>
              <Link href="/login" className="btn-accent btn-sm">
                Get started
              </Link>
            </>
          )}
          <button
            type="button"
            onClick={() => setOpen((o) => !o)}
            aria-label="Menu"
            aria-expanded={open}
            className="btn-ghost btn-sm md:hidden"
          >
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
              <path d="M2 4h12M2 8h12M2 12h12" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
            </svg>
          </button>
        </div>
      </div>

      {open ? (
        <div className="glass border-b px-6 py-3 md:hidden hairline">
          <nav className="flex flex-col gap-1">
            {NAV.map((item) => (
              <Link key={item.href} href={item.href} className="nav-link">
                {item.label}
              </Link>
            ))}
          </nav>
        </div>
      ) : null}
    </header>
  );
}
