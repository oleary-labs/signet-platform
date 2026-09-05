"use client";

import Link from "next/link";
import type { NodeOperator } from "@/lib/types";
import { formatDate, shortAddress } from "@/lib/format";
import { Badge, StatusDot } from "./ui";

const CATEGORY_LABELS: Record<NodeOperator["category"], string> = {
  signet: "Signet",
  enterprise: "Enterprise",
  infrastructure: "Infrastructure",
  custodian: "Custodian",
  independent: "Independent",
};

/** A monogram stands in for operators who have not supplied a logo, so the
 *  grid stays even instead of collapsing where an image is missing. */
function Monogram({ name }: { name: string }) {
  const initials = name
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase() ?? "")
    .join("");
  return (
    <div className="flex h-10 w-10 flex-none items-center justify-center rounded-lg bg-gradient-to-br from-primary-700 to-primary-500 text-[13px] font-bold text-white">
      {initials || "?"}
    </div>
  );
}

export function NodeCard({
  operator,
  selected,
  onToggle,
  href,
}: {
  operator: NodeOperator;
  selected?: boolean;
  onToggle?: () => void;
  href?: string;
}) {
  const health = operator.health;
  const body = (
    <>
      <div className="flex items-start gap-3.5">
        {operator.logo_url ? (
          // Operator logos are arbitrary developer-supplied URLs, so they use a
          // plain <img> rather than next/image, which needs a host allowlist.
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={operator.logo_url}
            alt=""
            className="h-10 w-10 flex-none rounded-lg object-cover"
          />
        ) : (
          <Monogram name={operator.name} />
        )}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h3 className="truncate text-[15px] font-semibold text-fg">{operator.name}</h3>
            {operator.verified ? (
              <span title="Reviewed by the Signet team" className="flex-none text-accent-500">
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                  <path
                    d="M8 1.5 9.9 3 12.3 2.8l.5 2.3 1.7 1.6-1.3 2 .6 2.3-2.3.7-1.3 2L8 12.9l-2.2.8-1.3-2-2.3-.7.6-2.3-1.3-2 1.7-1.6.5-2.3L6.1 3 8 1.5Z"
                    fill="currentColor"
                    fillOpacity="0.22"
                    stroke="currentColor"
                    strokeWidth="1.1"
                    strokeLinejoin="round"
                  />
                  <path d="m5.9 8 1.4 1.5L10.2 6.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
              </span>
            ) : null}
          </div>
          <p className="mt-0.5 text-[12.5px] text-muted">
            {CATEGORY_LABELS[operator.category]}
            {operator.jurisdiction ? ` · ${operator.jurisdiction}` : ""}
          </p>
        </div>
        {onToggle ? (
          <span
            className={`flex h-5 w-5 flex-none items-center justify-center rounded-md border transition ${
              selected ? "border-accent-500 bg-accent-500 text-white" : "border-fg/20"
            }`}
          >
            {selected ? (
              <svg width="11" height="11" viewBox="0 0 12 12" fill="none" aria-hidden="true">
                <path d="m2.5 6.2 2.3 2.3L9.5 3.8" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            ) : null}
          </span>
        ) : null}
      </div>

      {/* flex-1 so every card in a row ends at the same height, with the
          metadata footer aligned across the grid rather than floating. */}
      {operator.description ? (
        <p className="mt-3.5 line-clamp-3 flex-1 text-[13.5px] leading-relaxed text-muted">
          {operator.description}
        </p>
      ) : (
        <p className="mt-3.5 flex-1 text-[13.5px] italic leading-relaxed text-faint">
          This operator is registered on-chain but has not published a description yet.
        </p>
      )}

      <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 border-t pt-3.5 text-[12px] text-muted hairline">
        <span className="inline-flex items-center gap-1.5">
          {health ? (
            <>
              <StatusDot tone={health.online ? "success" : "error"} pulse={health.online} />
              {health.online ? "Online" : "Unreachable"}
              {health.latency_ms !== null && health.online ? (
                <span className="text-faint">· {health.latency_ms} ms</span>
              ) : null}
            </>
          ) : (
            <>
              <StatusDot tone="neutral" />
              <span className="text-faint">Not yet probed</span>
            </>
          )}
        </span>
        {health?.uptime_pct_24h !== null && health?.uptime_pct_24h !== undefined ? (
          <span title="Share of successful probes in the last 24 hours">
            {health.uptime_pct_24h.toFixed(1)}% / 24h
          </span>
        ) : null}
        <span>{operator.group_count} group{operator.group_count === 1 ? "" : "s"}</span>
        {operator.registered_at ? <span>Since {formatDate(operator.registered_at)}</span> : null}
      </div>

      <div className="mt-3 flex items-center justify-between gap-3">
        <span className="mono truncate text-faint">{shortAddress(operator.address, 10, 6)}</span>
        {operator.is_open === true ? (
          <Badge tone="success" dot>
            Open
          </Badge>
        ) : operator.is_open === false ? (
          <Badge tone="warn" dot>
            By invitation
          </Badge>
        ) : (
          <Badge tone="neutral">Not on chain</Badge>
        )}
      </div>
    </>
  );

  const shell = `card flex h-full flex-col p-5 text-left transition ${
    selected ? "ring-2 ring-accent-500/40 border-accent-500/60" : "hover:shadow-pop"
  }`;

  if (onToggle) {
    return (
      <button type="button" onClick={onToggle} className={shell}>
        {body}
      </button>
    );
  }
  if (href) {
    return (
      <Link href={href} className={shell}>
        {body}
      </Link>
    );
  }
  return <div className={shell}>{body}</div>;
}
