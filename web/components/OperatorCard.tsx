"use client";

import Link from "next/link";
import type { OperatorGroup } from "@/lib/operators";
import type { NodeOperator } from "@/lib/types";
import { shortAddress } from "@/lib/format";
import { Badge, StatusDot } from "./ui";

const CATEGORY_LABELS: Record<NodeOperator["category"], string> = {
  signet: "Signet",
  enterprise: "Enterprise",
  infrastructure: "Infrastructure",
  custodian: "Custodian",
  independent: "Independent",
};

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

/**
 * One operator, with the nodes it runs listed beneath it.
 *
 * The shape is the argument: the organisation is named once, and its nodes are
 * rows inside it. A grid of one card per node reads as that many independent
 * parties, which is the opposite of what choosing a threshold set requires —
 * four cards saying "O'Leary Labs" are one trust domain, and only the nesting
 * makes that legible at a glance.
 *
 * Each node still links to its own detail page: region, uptime and address are
 * per-node facts, and a developer comparing failure domains needs them.
 */
export function OperatorCard({ group }: { group: OperatorGroup }) {
  const nodeCount = group.nodes.length;

  return (
    <div className="card flex flex-col p-5">
      <div className="flex items-start gap-3.5">
        {group.logoUrl ? (
          // Operator logos are arbitrary developer-supplied URLs, so a plain
          // <img> rather than next/image, which needs a host allowlist.
          // eslint-disable-next-line @next/next/no-img-element
          <img src={group.logoUrl} alt="" className="h-10 w-10 flex-none rounded-lg object-cover" />
        ) : (
          <Monogram name={group.label} />
        )}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h3 className="truncate text-[15px] font-semibold text-fg">{group.label}</h3>
            {group.verified ? (
              <span title="Reviewed by the Signet team" className="flex-none text-accent-500">
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                  <path
                    d="M8 1.5 9.9 3 12.3 2.8l.5 2.3 1.7 1.6-1.3 2 .6 2.3-2.3.7-1.3 2L8 12.9l-2.2.8-1.3-2-2.3-.7.6-2.3-1.3-2 1.7-1.6.5-2.3L6.1 3 8 1.5Z"
                    fill="currentColor"
                  />
                </svg>
              </span>
            ) : null}
          </div>
          <p className="mt-0.5 truncate text-[12.5px] text-muted">
            {CATEGORY_LABELS[group.category]}
            {group.jurisdiction ? ` · ${group.jurisdiction}` : ""}
          </p>
        </div>
        <Badge tone={group.onlineCount === nodeCount ? "success" : "warn"}>
          {group.onlineCount}/{nodeCount} up
        </Badge>
      </div>

      {group.description ? (
        <p className="mt-3.5 line-clamp-3 text-[13.5px] leading-relaxed text-muted">
          {group.description}
        </p>
      ) : null}

      <div className="mt-4 border-t pt-3.5 hairline">
        <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
          {nodeCount} {nodeCount === 1 ? "node" : "nodes"}
        </p>
        <ul className="space-y-1.5">
          {group.nodes.map((node) => (
            <li key={node.address}>
              <Link
                href={`/marketplace/${node.address}`}
                className="flex items-center justify-between gap-3 rounded-lg px-1.5 py-1 transition hover:bg-fg/[0.04]"
              >
                <span className="flex min-w-0 items-center gap-2">
                  <StatusDot
                    tone={node.health?.online ? "success" : node.health ? "error" : "neutral"}
                  />
                  <span className="truncate text-[13px] text-fg">
                    {node.region || shortAddress(node.address)}
                  </span>
                </span>
                <span className="mono flex-none text-[12px] text-faint">
                  {shortAddress(node.address)}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </div>

      {group.websiteUrl ? (
        <a
          href={group.websiteUrl}
          target="_blank"
          rel="noreferrer noopener"
          className="btn-ghost mt-4 w-full"
        >
          Visit website
        </a>
      ) : null}
    </div>
  );
}
