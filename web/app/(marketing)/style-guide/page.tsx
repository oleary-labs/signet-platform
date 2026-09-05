"use client";

import { useState } from "react";
import { AreaChart, CHART_COLORS } from "@/components/Chart";
import { Logo, Wordmark } from "@/components/Logo";
import {
  Badge,
  Callout,
  CopyValue,
  EmptyState,
  PageHeader,
  Section,
  Skeleton,
  StatCard,
  StatusDot,
  Toggle,
} from "@/components/ui";

/**
 * The design system, on one page.
 *
 * It exists so the console and the marketing site cannot drift apart: every
 * token and component here is the same one the product uses, imported rather
 * than reproduced, so a change shows up here the moment it lands.
 */

const PALETTES = {
  Primary: {
    description:
      "Slate blue — trust and depth. Headings, body text, dark surfaces, primary buttons.",
    swatches: [
      ["primary-50", "#f0f4f8"], ["primary-100", "#d9e2ec"], ["primary-200", "#bcccdc"],
      ["primary-300", "#9fb3c8"], ["primary-400", "#829ab1"], ["primary-500", "#627d98"],
      ["primary-600", "#486581"], ["primary-700", "#334e68"], ["primary-800", "#243b53"],
      ["primary-900", "#102a43"], ["primary-950", "#0a1929"],
    ],
  },
  Accent: {
    description:
      "Sunset orange to burnt umber. CTAs, selected states, warnings. Used sparingly — it should catch the eye, not fill the page.",
    swatches: [
      ["accent-50", "#fff8f1"], ["accent-100", "#feecdc"], ["accent-200", "#fcd9bd"],
      ["accent-300", "#fdba8c"], ["accent-400", "#f6a354"], ["accent-500", "#e8873c"],
      ["accent-600", "#cb6d2a"], ["accent-700", "#a65521"], ["accent-800", "#8a4520"],
      ["accent-900", "#6f3720"], ["accent-950", "#3d1c0e"],
    ],
  },
  Neutral: {
    description:
      "Warm stone. Backgrounds, borders, secondary text — warm-tinted so it sits with the accent rather than fighting it.",
    swatches: [
      ["neutral-50", "#faf9f7"], ["neutral-100", "#f0eeeb"], ["neutral-200", "#e2dfd9"],
      ["neutral-300", "#ccc8c0"], ["neutral-400", "#aca69c"], ["neutral-500", "#918a7e"],
      ["neutral-600", "#756e63"], ["neutral-700", "#5e5850"], ["neutral-800", "#4a453f"],
      ["neutral-900", "#38342f"], ["neutral-950", "#1e1c19"],
    ],
  },
  Semantic: {
    description: "Status and feedback. Sage green for healthy, warm red for failure.",
    swatches: [
      ["success-400", "#4ade80"], ["success-500", "#3d9f6f"], ["success-600", "#2f8459"],
      ["success-700", "#236b47"], ["error-400", "#e8614d"], ["error-500", "#c4523b"],
      ["error-600", "#a8412e"],
    ],
  },
} as const;

export default function StyleGuidePage() {
  const [copied, setCopied] = useState<string | null>(null);
  const [toggle, setToggle] = useState(true);

  return (
    <div className="container-page py-14">
      <PageHeader
        eyebrow="Design system"
        title="Signet style guide"
      />

      <div className="space-y-8">
        <Section title="Mark">
          <div className="flex flex-wrap items-center gap-10">
            <div className="text-center">
              <Logo size={64} className="text-fg" />
              <p className="mt-3 text-[12px] text-muted">Mark</p>
            </div>
            <div className="text-center">
              <Wordmark className="text-fg" />
              <p className="mt-3 text-[12px] text-muted">Lockup</p>
            </div>
            <div className="rounded-2xl bg-primary-950 p-8 text-center">
              <Logo size={48} className="text-primary-100" />
              <p className="mt-3 text-[12px] text-primary-400">On dark</p>
            </div>
          </div>
          <p className="mt-6 max-w-prose text-[13.5px] leading-relaxed text-muted">
            No canonical logo existed in the Signet repositories, so this one is drawn from the
            protocol&rsquo;s own idea: a seal whose ring is six separate arcs. The ring is visibly
            not one piece, and one arc is picked out in the accent — the operator set is chosen, and
            no single arc is the seal.
          </p>
        </Section>

        <Section title="Colour">
          <div className="space-y-9">
            {Object.entries(PALETTES).map(([name, { description, swatches }]) => (
              <div key={name}>
                <h3 className="text-[15px] font-semibold text-fg">{name}</h3>
                <p className="mt-1 max-w-prose text-[13px] text-muted">{description}</p>
                <div className="mt-4 grid grid-cols-2 gap-2.5 sm:grid-cols-4 lg:grid-cols-6">
                  {swatches.map(([token, hex]) => (
                    <button
                      key={token}
                      type="button"
                      onClick={() => {
                        navigator.clipboard.writeText(hex);
                        setCopied(hex);
                        window.setTimeout(() => setCopied(null), 1200);
                      }}
                      className="overflow-hidden rounded-xl border text-left transition hover:shadow-card hairline"
                    >
                      <span className="flex h-16 items-end p-2" style={{ background: hex }}>
                        <span
                          className="font-mono text-[10px] font-medium"
                          style={{ color: isLight(hex) ? "#102a43" : "#ffffff", opacity: 0.75 }}
                        >
                          {copied === hex ? "copied" : hex}
                        </span>
                      </span>
                      <span className="block bg-surface px-2 py-1.5 text-[11px] font-medium text-fg">
                        {token}
                      </span>
                    </button>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </Section>

        <Section title="Typography">
          <div className="max-w-3xl space-y-6">
            {[
              ["text-[2.6rem] font-semibold tracking-[-0.03em]", "Page headline", "Inter · 42 / 600 / -0.03em"],
              ["text-2xl font-semibold tracking-tight", "Section heading", "Inter · 24 / 600"],
              ["text-[15px] font-semibold", "Subsection", "Inter · 15 / 600"],
              ["text-[15px] text-fg", "Body text. Signet splits the trust so you can keep the control.", "Inter · 15 / 400"],
              ["text-sm text-muted", "Secondary text for descriptions and helper copy.", "Inter · 14 / 400"],
              ["mono text-faint", "0x1234…abcd — JetBrains Mono for addresses, keys, and code", "JetBrains Mono · 12.5"],
            ].map(([cls, sample, meta]) => (
              <div key={meta} className="border-b pb-5 hairline">
                <p className="mb-2 font-mono text-[11px] text-faint">{meta}</p>
                <p className={cls}>{sample}</p>
              </div>
            ))}
          </div>
        </Section>

        <Section title="Buttons">
          <div className="space-y-6">
            <div>
              <p className="mb-3 text-[12px] font-semibold uppercase tracking-[0.08em] text-faint">
                On light
              </p>
              <div className="flex flex-wrap items-center gap-3">
                <button className="btn-accent">Accent</button>
                <button className="btn-primary">Primary</button>
                <button className="btn-ghost">Ghost</button>
                <button className="btn-quiet">Quiet</button>
                <button className="btn-danger">Danger</button>
                <button className="btn-accent" disabled>
                  Disabled
                </button>
                <button className="btn-ghost btn-sm">Small</button>
              </div>
            </div>
            <div className="rounded-2xl bg-primary-950 p-7">
              <p className="mb-3 text-[12px] font-semibold uppercase tracking-[0.08em] text-primary-400">
                On dark
              </p>
              <div className="flex flex-wrap items-center gap-3">
                <button className="btn-accent">Accent</button>
                <button className="btn border border-primary-600 text-primary-100 hover:border-primary-400">
                  Outline
                </button>
                <button className="btn text-primary-300 hover:text-primary-100">Ghost</button>
              </div>
            </div>
          </div>
        </Section>

        <Section title="Status">
          <div className="flex flex-wrap items-center gap-3">
            <Badge tone="success" dot>Operational</Badge>
            <Badge tone="success" dot>Open</Badge>
            <Badge tone="warn" dot>Pending</Badge>
            <Badge tone="error" dot>Below threshold</Badge>
            <Badge tone="accent">Scoped</Badge>
            <Badge tone="neutral">Unscoped</Badge>
          </div>
          <div className="mt-5 flex flex-wrap items-center gap-6 text-[13px] text-muted">
            <span className="inline-flex items-center gap-2">
              <StatusDot tone="success" pulse /> Live and healthy
            </span>
            <span className="inline-flex items-center gap-2">
              <StatusDot tone="warn" /> Waiting
            </span>
            <span className="inline-flex items-center gap-2">
              <StatusDot tone="error" /> Failing
            </span>
            <span className="inline-flex items-center gap-2">
              <StatusDot tone="neutral" /> Unknown
            </span>
          </div>
          <p className="mt-5 max-w-prose text-[13px] leading-relaxed text-muted">
            Green means healthy, orange means waiting on something, red means failing, grey means
            not yet known. Grey and red are kept distinct throughout: a node that has never been
            probed has not failed.
          </p>
        </Section>

        <Section title="Data display">
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <StatCard label="Active wallets" value="12,480" hint="Distinct users this period." />
            <StatCard label="Status" tone="success" value="Operational" hint="3-of-5, two can fail." />
            <StatCard label="Error rate" tone="error" value="6.2%" hint="Above the usual band." />
            <StatCard label="Configuration" value={<span className="mono text-[24px]">3-of-5</span>} />
          </div>

          <div className="mt-5">
            <AreaChart
              labels={["2026-08-25", "2026-08-26", "2026-08-27", "2026-08-28", "2026-08-29", "2026-08-30", "2026-08-31"]}
              series={[
                { key: "a", label: "Active wallets", color: CHART_COLORS.accent, values: [120, 180, 165, 240, 300, 280, 340] },
                { key: "b", label: "Signatures", color: CHART_COLORS.primary, values: [400, 520, 480, 700, 880, 810, 960] },
              ]}
            />
          </div>

          <div className="mt-6 max-w-md">
            <p className="label">Copyable value</p>
            <CopyValue value="0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266" />
          </div>
        </Section>

        <Section title="Forms">
          <div className="grid max-w-2xl gap-5">
            <div>
              <label className="label" htmlFor="demo-input">Text input</label>
              <input id="demo-input" className="input" placeholder="Acme Wallet" />
              <p className="hint">Helper text explaining what this field decides.</p>
            </div>
            <div>
              <label className="label" htmlFor="demo-select">Select</label>
              <select id="demo-select" className="select">
                <option>FROST · secp256k1</option>
                <option>ECDSA · secp256k1</option>
              </select>
            </div>
            <Toggle
              checked={toggle}
              onChange={setToggle}
              label="Require a scope on every delegated key"
            />
          </div>
        </Section>

        <Section title="Feedback">
          <div className="space-y-4">
            <Callout title="Neutral">
              For context the reader needs but does not have to act on.
            </Callout>
            <Callout tone="warn" title="Warning">
              For the gap between what the platform recorded and what the network will actually
              enforce. This is the most important callout in the product.
            </Callout>
            <Callout tone="error" title="Error">
              For a state that is actively broken — a group below its threshold, a failed deploy.
            </Callout>
            <Callout tone="success" title="Success">
              For a completed action worth confirming.
            </Callout>
          </div>

          <div className="mt-6 grid gap-4 lg:grid-cols-2">
            <EmptyState
              title="Nothing here yet"
              action={<button className="btn-accent btn-sm">Create one</button>}
            />
            <div className="space-y-2.5">
              <Skeleton className="h-12 rounded-xl" />
              <Skeleton className="h-12 rounded-xl" />
              <Skeleton className="h-12 rounded-xl" />
            </div>
          </div>
        </Section>

        <Section title="Motion">
          <p className="max-w-prose text-[13.5px] leading-relaxed text-muted">
            Reveals are driven by an IntersectionObserver, not a library:{" "}
            <code className="mono">.fx</code> rises and fades,{" "}
            <code className="mono">.fx-scale</code> also scales, and{" "}
            <code className="mono">.fx-d1</code> through <code className="mono">.fx-d5</code>{" "}
            stagger. Entrances that should run on load rather than on scroll use{" "}
            <code className="mono">.anim-rise</code>. Everything eases on{" "}
            <code className="mono">cubic-bezier(0.22, 1, 0.36, 1)</code>, and every animation
            collapses to nothing under <code className="mono">prefers-reduced-motion</code>.
          </p>
          <div className="mt-5 grid gap-3 sm:grid-cols-4">
            {[1, 2, 3, 4].map((i) => (
              <div key={i} className={`fx fx-d${i} card p-5 text-center text-[13px] text-muted`}>
                fx-d{i}
              </div>
            ))}
          </div>
        </Section>

        <Section title="Principles">
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {[
              ["Accent sparingly", "Sunset orange marks the one action on a screen that matters, plus selected and warning states. A page with three accent buttons has none."],
              ["Say what is enforced where", "The single most important thing this product communicates is the difference between a rule the operators check, a rule the chain checks, and a note to yourself. Never let those three look alike."],
              ["Errors carry their cause", "Show what the server actually said. A developer debugging a group has to see the message; a generic apology sends them to the logs."],
              ["Grey is not red", "Never probed is not offline. Not synced is not broken. Distinguish absence from failure everywhere."],
              ["Addresses are copyable", "Anything a developer might paste into a terminal gets a copy affordance, and is truncated the same way on every screen."],
              ["Motion explains, never decorates", "Reveals establish reading order and progress states show real work. Nothing moves to be interesting."],
            ].map(([title, body]) => (
              <div key={title} className="card-flat p-5">
                <h3 className="text-[14px] font-semibold text-fg">{title}</h3>
                <p className="mt-2 text-[13px] leading-relaxed text-muted">{body}</p>
              </div>
            ))}
          </div>
        </Section>
      </div>
    </div>
  );
}

/** Rough perceived lightness, to pick readable swatch text. */
function isLight(hex: string): boolean {
  const n = parseInt(hex.slice(1), 16);
  const [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255];
  return (0.299 * r + 0.587 * g + 0.114 * b) / 255 > 0.6;
}
