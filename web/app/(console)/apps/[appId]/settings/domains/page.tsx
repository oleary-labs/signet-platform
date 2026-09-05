"use client";

import { use, useState } from "react";
import {
  Callout,
  EmptyState,
  ErrorNote,
  PageHeader,
  Section,
  SkeletonRows,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useAction, useQuery } from "@/lib/hooks";
import { useToast } from "@/providers/ToastProvider";
import { formatDate } from "@/lib/format";

export default function DomainsPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);
  const toast = useToast();
  const domains = useQuery(() => api.domains(appId), [appId]);
  const [origin, setOrigin] = useState("");

  const [add, { pending, error }] = useAction(async () => {
    await api.addDomain(appId, origin.trim());
    setOrigin("");
    await domains.refresh();
    toast.success("Domain allowed");
  });

  const [remove] = useAction(async (id: string) => {
    await api.removeDomain(appId, id);
    await domains.refresh();
    toast.success("Domain removed");
  });

  return (
    <>
      <PageHeader
        title="Domains"
        info="An origin is a scheme and host — https://app.acme.dev, not https://app.acme.dev/login. Browsers report it without a path, so a stored path never matches."
      />

      <Section title="Allowed origins">
        <form
          className="mb-5 flex flex-wrap gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (origin.trim()) add();
          }}
        >
          <input
            className="input flex-1"
            placeholder="https://app.acme.dev"
            value={origin}
            onChange={(e) => setOrigin(e.target.value)}
            aria-label="Origin"
          />
          <button type="submit" className="btn-accent flex-none" disabled={pending || !origin.trim()}>
            {pending ? "Adding…" : "Add origin"}
          </button>
        </form>

        {error ? <ErrorNote error={error} /> : null}
        {domains.error ? <ErrorNote error={domains.error} onRetry={domains.refresh} /> : null}

        {domains.loading ? (
          <SkeletonRows rows={2} />
        ) : (domains.data ?? []).length === 0 ? (
          <EmptyState
            title="No origins allowed yet"
          />
        ) : (
          <ul className="space-y-2.5">
            {domains.data!.map((d) => (
              <li
                key={d.id}
                className="flex items-center justify-between gap-3 rounded-xl border px-4 py-3 hairline"
              >
                <div className="min-w-0">
                  <p className="mono truncate text-fg">{d.origin}</p>
                  <p className="mt-0.5 text-[12px] text-faint">Added {formatDate(d.created_at)}</p>
                </div>
                <button type="button" className="btn-danger btn-sm" onClick={() => remove(d.id)}>
                  Remove
                </button>
              </li>
            ))}
          </ul>
        )}

      </Section>
    </>
  );
}
