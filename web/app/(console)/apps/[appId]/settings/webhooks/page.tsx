"use client";

import { use, useState } from "react";
import {
  Badge,
  Callout,
  CopyValue,
  EmptyState,
  ErrorNote,
  InfoTip,
  Modal,
  PageHeader,
  Section,
  SkeletonRows,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useAction, useQuery } from "@/lib/hooks";
import { useToast } from "@/providers/ToastProvider";
import { formatDateTime, relativeTime } from "@/lib/format";
import type { Webhook } from "@/lib/types";

export default function WebhooksPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);
  const toast = useToast();
  const hooks = useQuery(() => api.webhooks(appId), [appId]);
  const events = useQuery(() => api.webhookEvents(), []);
  const [open, setOpen] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const [inspecting, setInspecting] = useState<Webhook | null>(null);

  const [remove] = useAction(async (id: string) => {
    await api.deleteWebhook(appId, id);
    await hooks.refresh();
    toast.success("Webhook deleted");
  });

  const [test] = useAction(async (id: string) => {
    const result = await api.testWebhook(appId, id);
    if (result.delivered) toast.success("Endpoint accepted the test", result.note);
    else toast.error("Endpoint rejected the test", result.error ?? result.note);
  });

  return (
    <>
      <PageHeader
        title="Webhooks"
        actions={
          <button type="button" className="btn-accent" onClick={() => setOpen(true)}>
            Add endpoint
          </button>
        }
      />

      <Section>
        {hooks.error ? <ErrorNote error={hooks.error} onRetry={hooks.refresh} /> : null}
        {hooks.loading ? (
          <SkeletonRows rows={2} />
        ) : (hooks.data ?? []).length === 0 ? (
          <EmptyState
            title="No endpoints yet"
            action={
              <button type="button" className="btn-accent" onClick={() => setOpen(true)}>
                Add endpoint
              </button>
            }
          />
        ) : (
          <ul className="space-y-2.5">
            {hooks.data!.map((h) => (
              <li key={h.id} className="rounded-xl border px-4 py-3.5 hairline">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="mono truncate text-fg">{h.url}</p>
                      <Badge tone={h.enabled ? "success" : "neutral"} dot>
                        {h.enabled ? "enabled" : "disabled"}
                      </Badge>
                    </div>
                    <p className="mt-1 text-[12.5px] text-muted">
                      {h.events.length === 0
                        ? "Subscribed to every event"
                        : `${h.events.length} event${h.events.length === 1 ? "" : "s"}: ${h.events.slice(0, 3).join(", ")}${h.events.length > 3 ? "…" : ""}`}
                    </p>
                  </div>
                  <div className="flex flex-none gap-2">
                    <button type="button" className="btn-ghost btn-sm" onClick={() => setInspecting(h)}>
                      Deliveries
                    </button>
                    <button type="button" className="btn-ghost btn-sm" onClick={() => test(h.id)}>
                      Send test
                    </button>
                    <button type="button" className="btn-danger btn-sm" onClick={() => remove(h.id)}>
                      Delete
                    </button>
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Section>

      <div className="mt-5">
        <Section title="Verifying a delivery">
          <p className="mb-4 text-[13.5px] leading-relaxed text-muted">
            Every request carries <code>Signet-Timestamp</code> and <code>Signet-Signature</code>.
            The signature covers the timestamp and the body together, so a captured delivery cannot
            be replayed later with a fresh timestamp. Compare it in constant time, and reject
            anything older than a few minutes.
          </p>
          <div className="code-block">
            <div className="code-title">
              <span>verify.ts</span>
            </div>
            <pre>{`import { createHmac, timingSafeEqual } from "node:crypto";

export function verify(secret: string, headers: Headers, rawBody: string) {
  const timestamp = headers.get("signet-timestamp") ?? "";
  const signature = headers.get("signet-signature") ?? "";

  // Reject stale deliveries outright — the timestamp is signed, but an
  // attacker can still resend a genuine old one.
  const age = Math.abs(Date.now() / 1000 - Number(timestamp));
  if (!timestamp || age > 300) return false;

  const mac = createHmac("sha256", secret);
  mac.update(timestamp);
  mac.update(".");
  mac.update(rawBody);
  const expected = \`v1=\${mac.digest("hex")}\`;

  const a = Buffer.from(expected);
  const b = Buffer.from(signature);
  return a.length === b.length && timingSafeEqual(a, b);
}`}</pre>
          </div>
        </Section>
      </div>

      <NewWebhookModal
        open={open}
        onClose={() => setOpen(false)}
        appId={appId}
        events={events.data?.events ?? []}
        onCreated={(s) => {
          setOpen(false);
          hooks.refresh();
          setSecret(s);
        }}
      />

      <Modal
        open={secret !== null}
        onClose={() => setSecret(null)}
        title="Signing secret"
        description="Shown once. Store it wherever your endpoint reads its configuration from."
        footer={
          <button type="button" className="btn-accent" onClick={() => setSecret(null)}>
            I have saved it
          </button>
        }
      >
        <div className="rounded-xl border border-accent-500/40 bg-accent-500/[0.07] p-4">
          <CopyValue value={secret ?? ""} className="w-full" />
        </div>
      </Modal>

      <DeliveriesModal
        webhook={inspecting}
        appId={appId}
        onClose={() => setInspecting(null)}
      />
    </>
  );
}

function NewWebhookModal({
  open,
  onClose,
  appId,
  events,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  appId: string;
  events: string[];
  onCreated: (secret: string) => void;
}) {
  const [url, setUrl] = useState("");
  const [selected, setSelected] = useState<string[]>([]);

  const [create, { pending, error }] = useAction(async () => {
    const hook = await api.createWebhook(appId, { url: url.trim(), events: selected });
    setUrl("");
    setSelected([]);
    onCreated(hook.secret ?? "");
  });

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Add a webhook endpoint"
      wide
      footer={
        <>
          <button type="button" className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            type="button"
            className="btn-accent"
            disabled={pending || !url.trim()}
            onClick={() => create()}
          >
            {pending ? "Creating…" : "Create endpoint"}
          </button>
        </>
      }
    >
      {error ? <ErrorNote error={error} /> : null}

      <label className="label mt-4" htmlFor="url">
        Endpoint URL
      </label>
      <input
        id="url"
        className="input"
        placeholder="https://api.acme.dev/webhooks/signet"
        value={url}
        onChange={(e) => setUrl(e.target.value)}
      />

      <div className="mt-5">
        <span className="label">Events</span>
        <p className="mb-3 text-[12.5px] text-muted">
          Leave everything unselected to receive all events, including any added later.
        </p>
        <div className="grid max-h-64 gap-1.5 overflow-y-auto sm:grid-cols-2">
          {events.map((e) => (
            <label key={e} className="flex cursor-pointer items-center gap-2 text-[13px] text-muted">
              <input
                type="checkbox"
                checked={selected.includes(e)}
                onChange={(ev) =>
                  setSelected((prev) =>
                    ev.target.checked ? [...prev, e] : prev.filter((x) => x !== e),
                  )
                }
                className="accent-[#e8873c]"
              />
              <span className="mono">{e}</span>
            </label>
          ))}
        </div>
      </div>
    </Modal>
  );
}

function DeliveriesModal({
  webhook,
  appId,
  onClose,
}: {
  webhook: Webhook | null;
  appId: string;
  onClose: () => void;
}) {
  const deliveries = useQuery(
    () => (webhook ? api.deliveries(appId, webhook.id) : Promise.resolve([])),
    [webhook?.id],
  );

  if (!webhook) return null;

  return (
    <Modal open onClose={onClose} title="Recent deliveries" wide description={webhook.url}>
      {deliveries.loading ? (
        <SkeletonRows rows={4} />
      ) : (deliveries.data ?? []).length === 0 ? (
        <EmptyState
          title="No deliveries yet"
        />
      ) : (
        <div className="table-wrap max-h-[55vh] overflow-y-auto">
          <table className="table">
            <thead>
              <tr>
                <th>Event</th>
                <th>Result</th>
                <th>Took</th>
                <th>When</th>
              </tr>
            </thead>
            <tbody>
              {deliveries.data!.map((d) => (
                <tr key={d.id}>
                  <td className="mono text-muted">{d.event}</td>
                  <td>
                    {d.error ? (
                      <span title={d.error}>
                        <Badge tone="error">{d.status_code ?? "failed"}</Badge>
                      </span>
                    ) : (
                      <Badge tone="success">{d.status_code}</Badge>
                    )}
                  </td>
                  <td className="text-[13px] text-muted">
                    {d.duration_ms !== null ? `${d.duration_ms} ms` : "—"}
                  </td>
                  <td className="whitespace-nowrap text-[13px] text-muted" title={formatDateTime(d.created_at)}>
                    {relativeTime(d.created_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <Callout
        title={
          <>
            Deliveries are not retried
            <InfoTip>
              A failed delivery is recorded and left alone. Reconcile against the API for anything
              you cannot miss.
            </InfoTip>
          </>
        }
      />
    </Modal>
  );
}
