"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { ThemeToggle } from "@/components/ThemeToggle";
import { useSession } from "@/providers/SessionProvider";
import { shortAddress } from "@/lib/format";

export function UserMenu() {
  const { user, signOut } = useSession();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const router = useRouter();

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  if (!user) return null;

  const label = user.display_name || user.email || shortAddress(user.account_address ?? user.subject, 8, 6);

  return (
    <div ref={ref} className="relative flex items-center gap-1">
      <ThemeToggle />
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex items-center gap-2 rounded-lg px-2 py-1.5 text-[13px] text-muted transition hover:bg-fg/[0.05] hover:text-fg"
      >
        <span className="flex h-6 w-6 items-center justify-center rounded-full bg-gradient-to-br from-accent-500 to-accent-700 text-[10px] font-bold text-white">
          {label.slice(0, 2).toUpperCase()}
        </span>
        <span className="hidden max-w-[160px] truncate sm:inline">{label}</span>
      </button>

      {open ? (
        <div className="absolute right-0 top-full z-30 mt-1.5 w-64 rounded-xl border bg-surface p-1.5 shadow-pop hairline">
          <div className="px-2.5 py-2">
            <p className="truncate text-[13px] font-semibold text-fg">{label}</p>
            <p className="mt-0.5 truncate text-[11.5px] text-faint">
              {user.subject_kind === "signet" ? "Signet account" : "Ethereum account"}
            </p>
          </div>
          <div className="my-1 border-t hairline" />
          <Link
            href="/account"
            onClick={() => setOpen(false)}
            className="block rounded-lg px-2.5 py-2 text-[13px] text-muted transition hover:bg-fg/[0.05] hover:text-fg"
          >
            Account settings
          </Link>
          <Link
            href="/docs"
            onClick={() => setOpen(false)}
            className="block rounded-lg px-2.5 py-2 text-[13px] text-muted transition hover:bg-fg/[0.05] hover:text-fg"
          >
            Documentation
          </Link>
          <div className="my-1 border-t hairline" />
          <button
            type="button"
            onClick={async () => {
              setOpen(false);
              await signOut();
              router.replace("/");
            }}
            className="block w-full rounded-lg px-2.5 py-2 text-left text-[13px] text-muted transition hover:bg-fg/[0.05] hover:text-fg"
          >
            Sign out
          </button>
        </div>
      ) : null}
    </div>
  );
}
