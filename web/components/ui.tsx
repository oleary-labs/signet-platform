"use client";

import Link from "next/link";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";

/* ────────────────────────────── Status ────────────────────────────── */

export type Tone = "success" | "warn" | "error" | "neutral" | "accent";

const TONE_CLASSES: Record<Tone, string> = {
  success: "bg-success-500/12 text-success-700 dark:text-success-400",
  warn: "bg-accent-500/14 text-accent-700 dark:text-accent-300",
  error: "bg-error-500/12 text-error-600 dark:text-error-400",
  neutral: "bg-fg/[0.06] text-muted",
  accent: "bg-accent-500/14 text-accent-700 dark:text-accent-300",
};

const DOT_CLASSES: Record<Tone, string> = {
  success: "bg-success-500",
  warn: "bg-accent-500",
  error: "bg-error-500",
  neutral: "bg-faint",
  accent: "bg-accent-500",
};

export function Badge({
  tone = "neutral",
  dot = false,
  children,
}: {
  tone?: Tone;
  dot?: boolean;
  children: ReactNode;
}) {
  return (
    <span className={`tag ${TONE_CLASSES[tone]}`}>
      {dot ? <span className={`h-1.5 w-1.5 rounded-full ${DOT_CLASSES[tone]}`} /> : null}
      {children}
    </span>
  );
}

export function StatusDot({ tone, pulse = false }: { tone: Tone; pulse?: boolean }) {
  if (pulse && tone === "success") return <span className="pulse-dot" />;
  return <span className={`h-2 w-2 flex-none rounded-full ${DOT_CLASSES[tone]}`} />;
}

/* ────────────────────────────── Copy ────────────────────────────── */

/**
 * A value with a copy button. Addresses, key IDs, and hashes are things
 * developers paste into terminals, so every one of them is copyable rather
 * than something to select by hand.
 */
export function CopyValue({
  value,
  display,
  className = "",
  label,
}: {
  value: string;
  display?: string;
  className?: string;
  label?: string;
}) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | undefined>(undefined);

  useEffect(() => () => window.clearTimeout(timer.current), []);

  const copy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.clearTimeout(timer.current);
      timer.current = window.setTimeout(() => setCopied(false), 1400);
    } catch {
      // Clipboard access can be denied; the value is still on screen.
    }
  }, [value]);

  return (
    <button
      type="button"
      onClick={copy}
      title={label ? `${label}: ${value}` : value}
      className={`group inline-flex max-w-full items-center gap-1.5 rounded-md px-1.5 py-0.5 text-left transition hover:bg-fg/[0.06] ${className}`}
    >
      <span className="mono truncate">{display ?? value}</span>
      <span
        className={`flex-none text-[11px] font-semibold uppercase tracking-wide transition ${
          copied ? "text-success-600" : "text-faint opacity-0 group-hover:opacity-100"
        }`}
      >
        {copied ? "copied" : "copy"}
      </span>
    </button>
  );
}

/* ────────────────────────────── Tooltip ────────────────────────────── */

/**
 * An information affordance.
 *
 * The console is terse by default; anything a developer genuinely needs in
 * order to act correctly lives in one of these rather than in a paragraph they
 * would scroll past. It opens on hover *and* focus, and the content is in the
 * DOM with `role="tooltip"`, so it is reachable by keyboard and by a screen
 * reader — a mouse-only hint is the same as no hint for some people.
 */
export function InfoTip({
  children,
  label = "More information",
  side = "top",
}: {
  children: ReactNode;
  label?: string;
  side?: "top" | "bottom" | "right";
}) {
  const [open, setOpen] = useState(false);
  const position =
    side === "bottom"
      ? "top-full mt-2 left-1/2 -translate-x-1/2"
      : side === "right"
        ? "left-full ml-2 top-1/2 -translate-y-1/2"
        : "bottom-full mb-2 left-1/2 -translate-x-1/2";

  return (
    <span className="relative inline-flex align-middle">
      <button
        type="button"
        aria-label={label}
        aria-expanded={open}
        onMouseEnter={() => setOpen(true)}
        onMouseLeave={() => setOpen(false)}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onClick={(e) => {
          e.preventDefault();
          setOpen((o) => !o);
        }}
        className="inline-flex h-[15px] w-[15px] flex-none items-center justify-center rounded-full border text-[10px] font-semibold leading-none text-faint transition hover:border-accent-500 hover:text-accent-600"
        style={{ borderColor: "rgb(var(--edge) / 0.25)" }}
      >
        i
      </button>
      {open ? (
        <span
          role="tooltip"
          className={`absolute z-50 w-64 rounded-lg border bg-surface px-3 py-2 text-left text-[12.5px] font-normal leading-relaxed text-muted shadow-pop hairline ${position}`}
        >
          {children}
        </span>
      ) : null}
    </span>
  );
}

/* ────────────────────────────── Layout ────────────────────────────── */

export function PageHeader({
  title,
  info,
  actions,
  eyebrow,
}: {
  title: string;
  /** Crucial context, shown in a tooltip rather than as a subtitle. */
  info?: ReactNode;
  actions?: ReactNode;
  eyebrow?: string;
}) {
  return (
    <header className="mb-6 flex flex-wrap items-center justify-between gap-4">
      <div className="min-w-0">
        {eyebrow ? (
          <p className="mb-1 text-[11px] font-semibold uppercase tracking-[0.12em] text-accent-600 dark:text-accent-400">
            {eyebrow}
          </p>
        ) : null}
        <h1 className="flex items-center gap-2 text-2xl font-semibold tracking-tight text-fg">
          {title}
          {info ? <InfoTip side="bottom">{info}</InfoTip> : null}
        </h1>
      </div>
      {actions ? <div className="flex flex-none items-center gap-2">{actions}</div> : null}
    </header>
  );
}

export function Section({
  title,
  info,
  actions,
  children,
  className = "",
}: {
  title?: string;
  /** Crucial context, shown in a tooltip rather than as a paragraph. */
  info?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`card p-5 sm:p-6 ${className}`}>
      {(title || actions) && (
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            {title ? (
              <h2 className="section-title flex items-center gap-2">
                {title}
                {info ? <InfoTip>{info}</InfoTip> : null}
              </h2>
            ) : null}
          </div>
          {actions ? <div className="flex flex-none items-center gap-2">{actions}</div> : null}
        </div>
      )}
      {children}
    </section>
  );
}

export function StatCard({
  label,
  value,
  hint,
  tone,
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  tone?: Tone;
}) {
  return (
    <div className="card-flat p-4">
      <p className="text-[11px] font-semibold uppercase tracking-[0.09em] text-faint">{label}</p>
      <div
        className={`mt-1.5 text-[26px] font-semibold leading-none tracking-tight ${
          tone === "success"
            ? "text-success-600 dark:text-success-400"
            : tone === "error"
              ? "text-error-500"
              : tone === "accent"
                ? "text-accent-600 dark:text-accent-400"
                : "text-fg"
        }`}
      >
        {value}
      </div>
      {hint ? <p className="mt-2 text-[12.5px] leading-snug text-muted">{hint}</p> : null}
    </div>
  );
}

export function EmptyState({
  title,
  info,
  action,
  icon,
}: {
  title: string;
  /** Why it is empty and what fills it, in a tooltip. */
  info?: ReactNode;
  action?: ReactNode;
  icon?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed px-6 py-12 text-center hairline">
      {icon ? <div className="mb-3 text-faint">{icon}</div> : null}
      <p className="flex items-center gap-2 text-[14px] font-medium text-fg">
        {title}
        {info ? <InfoTip>{info}</InfoTip> : null}
      </p>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

export function Skeleton({ className = "h-4 w-full" }: { className?: string }) {
  return <div className={`skeleton ${className}`} />;
}

export function SkeletonRows({ rows = 4 }: { rows?: number }) {
  return (
    <div className="space-y-2.5">
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton key={i} className="h-12 w-full rounded-xl" />
      ))}
    </div>
  );
}

/**
 * An inline error with the real message.
 *
 * Failures show what the server actually said rather than a generic apology —
 * a developer debugging a misconfigured group needs the detail, and hiding it
 * only sends them to the logs.
 */
export function ErrorNote({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const message = error instanceof Error ? error.message : String(error);
  return (
    <div className="rounded-xl border border-error-500/30 bg-error-500/[0.07] px-4 py-3">
      <p className="text-sm font-semibold text-error-600 dark:text-error-400">
        Something went wrong
      </p>
      <p className="mt-1 break-words text-[13px] leading-relaxed text-error-600/90 dark:text-error-400/90">
        {message}
      </p>
      {onRetry ? (
        <button type="button" onClick={onRetry} className="btn-ghost btn-sm mt-3">
          Try again
        </button>
      ) : null}
    </div>
  );
}

/**
 * A callout for statements the console needs to make plainly — most often
 * where the platform's record and the chain's state are different things.
 */
export function Callout({
  tone = "neutral",
  title,
  children,
}: {
  tone?: Tone;
  /** A short line. Anything longer belongs in an InfoTip beside it, which is
   *  why this takes a node rather than a string. */
  title?: ReactNode;
  children?: ReactNode;
}) {
  const border =
    tone === "warn" || tone === "accent"
      ? "border-accent-500/30 bg-accent-500/[0.07]"
      : tone === "error"
        ? "border-error-500/30 bg-error-500/[0.07]"
        : tone === "success"
          ? "border-success-500/30 bg-success-500/[0.07]"
          : "hairline bg-fg/[0.03]";
  return (
    <div className={`rounded-xl border px-3.5 py-2.5 ${border}`}>
      {title ? (
        <p className="flex items-center gap-1.5 text-[12.5px] font-semibold text-fg">{title}</p>
      ) : null}
      {children ? (
        <div className={`text-[12.5px] leading-relaxed text-muted ${title ? "mt-1.5" : ""}`}>
          {children}
        </div>
      ) : null}
    </div>
  );
}

/* ────────────────────────────── Modal ────────────────────────────── */

export function Modal({
  open,
  onClose,
  title,
  description,
  info,
  children,
  footer,
  wide = false,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  /** One short line, when the title alone is ambiguous. Anything longer goes
   *  in `info`, where it is available without becoming a wall to scroll past. */
  description?: ReactNode;
  info?: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  wide?: boolean;
}) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    // Freeze the page behind the dialog so a long form does not scroll the
    // list underneath it.
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = previous;
    };
  }, [open, onClose]);

  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-primary-950/45 p-4 backdrop-blur-sm sm:p-8">
      <div
        className="absolute inset-0"
        onClick={onClose}
        aria-hidden="true"
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className={`anim-rise relative z-10 w-full ${wide ? "max-w-3xl" : "max-w-lg"} rounded-2xl border bg-surface p-6 shadow-pop hairline`}
      >
        <div className="mb-5">
          <h2 className="flex items-center gap-1.5 text-lg font-semibold tracking-tight text-fg">
            {title}
            {info ? <InfoTip>{info}</InfoTip> : null}
          </h2>
          {description ? (
            <div className="mt-1.5 text-sm leading-relaxed text-muted">{description}</div>
          ) : null}
        </div>
        {children}
        {footer ? <div className="mt-6 flex justify-end gap-2">{footer}</div> : null}
      </div>
    </div>
  );
}

/* ────────────────────────────── Controls ────────────────────────────── */

export function Toggle({
  checked,
  onChange,
  label,
  info,
  disabled,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: ReactNode;
  /** Crucial context, in a tooltip rather than a second line. */
  info?: ReactNode;
  disabled?: boolean;
}) {
  return (
    <label
      className={`flex items-center justify-between gap-4 rounded-xl border px-4 py-2.5 transition hairline ${
        disabled ? "opacity-55" : "cursor-pointer hover:bg-fg/[0.03]"
      }`}
    >
      <span className="flex min-w-0 items-center gap-2">
        <span className="truncate text-sm text-fg">{label}</span>
        {info ? <InfoTip>{info}</InfoTip> : null}
      </span>
      <span className="relative flex-none">
        <input
          type="checkbox"
          className="peer sr-only"
          checked={checked}
          disabled={disabled}
          onChange={(e) => onChange(e.target.checked)}
        />
        <span
          className={`block h-[22px] w-[38px] rounded-full transition ${
            checked ? "bg-accent-500" : "bg-fg/15"
          }`}
        />
        <span
          className={`absolute left-[3px] top-[3px] block h-4 w-4 rounded-full bg-white shadow transition ${
            checked ? "translate-x-4" : ""
          }`}
        />
      </span>
    </label>
  );
}

export function Tabs({
  tabs,
  active,
  onChange,
}: {
  tabs: { id: string; label: string; count?: number }[];
  active: string;
  onChange: (id: string) => void;
}) {
  return (
    <div className="flex gap-1 overflow-x-auto border-b pb-px hairline">
      {tabs.map((t) => (
        <button
          key={t.id}
          type="button"
          onClick={() => onChange(t.id)}
          className={`-mb-px whitespace-nowrap border-b-2 px-3.5 py-2.5 text-[13.5px] font-medium transition ${
            active === t.id
              ? "border-accent-500 text-fg"
              : "border-transparent text-muted hover:text-fg"
          }`}
        >
          {t.label}
          {t.count !== undefined ? (
            <span className="ml-1.5 text-[11px] text-faint">{t.count}</span>
          ) : null}
        </button>
      ))}
    </div>
  );
}

export function LinkButton({
  href,
  children,
  variant = "ghost",
  className = "",
}: {
  href: string;
  children: ReactNode;
  variant?: "primary" | "accent" | "ghost";
  className?: string;
}) {
  const cls =
    variant === "primary" ? "btn-primary" : variant === "accent" ? "btn-accent" : "btn-ghost";
  const external = href.startsWith("http");
  if (external) {
    return (
      <a href={href} target="_blank" rel="noreferrer" className={`${cls} ${className}`}>
        {children}
      </a>
    );
  }
  return (
    <Link href={href} className={`${cls} ${className}`}>
      {children}
    </Link>
  );
}
