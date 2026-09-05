"use client";

import { useMemo, useState } from "react";
import { NodeCard } from "@/components/NodeCard";
import { EmptyState, ErrorNote, PageHeader, Skeleton } from "@/components/ui";
import { useQuery } from "@/lib/hooks";
import { api } from "@/lib/api";
import type { NodeOperator } from "@/lib/types";

const CATEGORIES = [
  { id: "", label: "All" },
  { id: "signet", label: "Signet" },
  { id: "enterprise", label: "Enterprise" },
  { id: "infrastructure", label: "Infrastructure" },
  { id: "custodian", label: "Custodian" },
  { id: "independent", label: "Independent" },
];

type SortKey = "name" | "uptime" | "groups" | "newest";

export default function MarketplacePage() {
  const [category, setCategory] = useState("");
  const [search, setSearch] = useState("");
  const [openOnly, setOpenOnly] = useState(false);
  const [onlineOnly, setOnlineOnly] = useState(false);
  const [sort, setSort] = useState<SortKey>("uptime");

  const query = useQuery(() => api.nodeOperators(), []);

  const operators = useMemo(() => {
    const list = (query.data ?? []).filter((o) => {
      if (category && o.category !== category) return false;
      if (openOnly && o.is_open !== true) return false;
      if (onlineOnly && !o.health?.online) return false;
      if (search) {
        const q = search.toLowerCase();
        if (
          !o.name.toLowerCase().includes(q) &&
          !o.description.toLowerCase().includes(q) &&
          !o.jurisdiction.toLowerCase().includes(q) &&
          !o.address.toLowerCase().includes(q)
        ) {
          return false;
        }
      }
      return true;
    });
    return sortOperators(list, sort);
  }, [query.data, category, openOnly, onlineOnly, search, sort]);

  return (
    <div className="container-page py-14">
      <PageHeader
        eyebrow="Operator marketplace"
        title="Choose who signs for your users"
      />

      <div className="mb-7 space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <input
            className="input w-full sm:w-64"
            placeholder="Search operators…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            aria-label="Search operators"
          />
          <select
            className="select w-auto"
            value={sort}
            onChange={(e) => setSort(e.target.value as SortKey)}
            aria-label="Sort operators"
          >
            <option value="uptime">Sort: uptime</option>
            <option value="name">Sort: name</option>
            <option value="groups">Sort: groups served</option>
            <option value="newest">Sort: newest</option>
          </select>
          <label className="ml-auto flex cursor-pointer items-center gap-2 text-[13px] text-muted">
            <input
              type="checkbox"
              checked={openOnly}
              onChange={(e) => setOpenOnly(e.target.checked)}
              className="accent-[#e8873c]"
            />
            Open only
          </label>
          <label className="flex cursor-pointer items-center gap-2 text-[13px] text-muted">
            <input
              type="checkbox"
              checked={onlineOnly}
              onChange={(e) => setOnlineOnly(e.target.checked)}
              className="accent-[#e8873c]"
            />
            Online only
          </label>
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-3 hairline">
          <div className="flex flex-wrap gap-1.5">
            {CATEGORIES.map((c) => (
              <button
                key={c.id}
                type="button"
                onClick={() => setCategory(c.id)}
                className={`rounded-lg px-3 py-1.5 text-[13px] font-medium transition ${
                  category === c.id
                    ? "bg-accent-500/14 text-accent-700 dark:text-accent-300"
                    : "text-muted hover:bg-fg/[0.05] hover:text-fg"
                }`}
              >
                {c.label}
              </button>
            ))}
          </div>
          <p className="text-[12.5px] text-faint">
            {operators.length} of {query.data?.length ?? 0} operators
          </p>
        </div>
      </div>

      {query.error ? <ErrorNote error={query.error} onRetry={query.refresh} /> : null}

      {query.loading ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-[236px] rounded-2xl" />
          ))}
        </div>
      ) : operators.length === 0 ? (
        <EmptyState
          title="No operators match those filters"
        />
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {operators.map((o, i) => (
            <div key={o.address} className={`fx fx-d${(i % 4) + 1}`}>
              <NodeCard operator={o} href={`/marketplace/${o.address}`} />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function sortOperators(list: NodeOperator[], sort: SortKey): NodeOperator[] {
  const copy = [...list];
  switch (sort) {
    case "name":
      return copy.sort((a, b) => a.name.localeCompare(b.name));
    case "groups":
      return copy.sort((a, b) => b.group_count - a.group_count);
    case "newest":
      return copy.sort(
        (a, b) =>
          new Date(b.registered_at ?? 0).getTime() - new Date(a.registered_at ?? 0).getTime(),
      );
    case "uptime":
    default:
      // Verified operators lead, then live ones, then by 24-hour uptime. An
      // operator that has never been probed sorts below one that has, rather
      // than above it on a missing value.
      return copy.sort((a, b) => {
        if (a.verified !== b.verified) return a.verified ? -1 : 1;
        const aUp = a.health?.online ? 1 : 0;
        const bUp = b.health?.online ? 1 : 0;
        if (aUp !== bUp) return bUp - aUp;
        return (b.health?.uptime_pct_24h ?? -1) - (a.health?.uptime_pct_24h ?? -1);
      });
  }
}
