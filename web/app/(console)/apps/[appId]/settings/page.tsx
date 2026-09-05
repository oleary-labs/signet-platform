"use client";

import { useRouter } from "next/navigation";
import { use, useEffect, useState } from "react";
import Link from "next/link";
import {
  Callout,
  CopyValue,
  ErrorNote,
  InfoTip,
  Modal,
  PageHeader,
  Section,
  SkeletonRows,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useAction, useQuery } from "@/lib/hooks";
import { useSession } from "@/providers/SessionProvider";
import { useToast } from "@/providers/ToastProvider";
import { chainName, formatDateTime } from "@/lib/format";

export default function AppSettingsPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);
  const router = useRouter();
  const toast = useToast();
  const { refresh } = useSession();

  const app = useQuery(() => api.app(appId), [appId]);
  const prodCheck = useQuery(() => api.environmentCheck(appId, "production"), [appId]);
  const orgId = app.data?.org_id ?? null;
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [environment, setEnvironment] = useState<"development" | "production">("development");
  const [website, setWebsite] = useState("");
  const [archiveOpen, setArchiveOpen] = useState(false);
  const [confirmName, setConfirmName] = useState("");

  useEffect(() => {
    if (!app.data) return;
    setName(app.data.name);
    setDescription(app.data.description);
    setEnvironment(app.data.environment);
    setWebsite(app.data.website_url ?? "");
  }, [app.data]);

  const [save, { pending, error }] = useAction(async () => {
    await api.updateApp(appId, {
      name,
      description,
      environment,
      website_url: website,
    });
    await app.refresh();
    await prodCheck.refresh();
    await refresh();
    toast.success("Saved");
  });

  const [archive, { pending: archiving }] = useAction(async () => {
    const result = await api.archiveApp(appId);
    await refresh();
    toast.info("App archived", result.note);
    router.push("/apps");
  });

  if (app.error) return <ErrorNote error={app.error} onRetry={app.refresh} />;

  return (
    <>
      <PageHeader
        title="Settings"
        info="Archiving removes the app from your console. It does not touch the chain — your signing group keeps running until you remove its operators yourself." />

      <div className="grid items-start gap-5 lg:grid-cols-3">
        <div className="space-y-5 lg:col-span-2">
          <Section title="Basics">
            <form
              className="space-y-5"
              onSubmit={(e) => {
                e.preventDefault();
                save();
              }}
            >
              <div>
                <label className="label" htmlFor="name">
                  App name
                </label>
                <input id="name" className="input" value={name} onChange={(e) => setName(e.target.value)} />
              </div>
              <div>
                <label className="label" htmlFor="description">
                  Description
                </label>
                <textarea
                  id="description"
                  className="textarea"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </div>
              <div>
                <label className="label" htmlFor="website">
                  Website
                </label>
                <input
                  id="website"
                  className="input"
                  placeholder="https://acme.dev"
                  value={website}
                  onChange={(e) => setWebsite(e.target.value)}
                />
              </div>
              <div>
                <span className="label">Environment</span>
                <div className="grid gap-2 sm:grid-cols-3">
                  {(["development", "production"] as const).map((env) => {
                    const blocked = env === "production" && prodCheck.data?.allowed === false;
                    return (
                      <button
                        key={env}
                        type="button"
                        onClick={() => setEnvironment(env)}
                        disabled={blocked}
                        title={blocked ? "Requirements below are not met yet" : undefined}
                        className={`rounded-xl border px-3.5 py-2.5 text-[13.5px] capitalize transition ${
                          environment === env
                            ? "border-accent-500 bg-accent-500/[0.08] text-fg"
                            : blocked
                              ? "cursor-not-allowed text-faint hairline"
                              : "text-muted hover:bg-fg/[0.03] hairline"
                        }`}
                      >
                        {env}
                      </button>
                    );
                  })}
                </div>
              </div>

              {error ? <ErrorNote error={error} /> : null}
              <button type="submit" className="btn-accent" disabled={pending}>
                {pending ? "Saving…" : "Save changes"}
              </button>
            </form>
          </Section>

          <Section
            title="Danger zone"
          >
            <Callout
              tone="warn"
              title={
                <>
                  Archiving does not stop your signing group
                  <InfoTip>
                    Its operators keep their shares and keep serving requests until you remove them
                    yourself — a queued, timelocked removal each, from your own account. Archiving is
                    bookkeeping, not a shutdown.
                  </InfoTip>
                </>
              }
            />
            <button type="button" className="btn-danger mt-4" onClick={() => setArchiveOpen(true)}>
              Archive this app
            </button>
          </Section>
        </div>

        <div className="space-y-5">
          <Section
            title="Going to production"
          >
            {prodCheck.loading ? (
              <SkeletonRows rows={3} />
            ) : prodCheck.data ? (
              <>
                {app.data?.environment === "production" ? (
                  <Callout
                    tone="success"
                    title={
                      <>
                        In production
                        <InfoTip>
                          These requirements were met at promotion. Removing operators can take a
                          group back below them.
                        </InfoTip>
                      </>
                    }
                  />
                ) : null}

                <ul className="space-y-2.5">
                  {(prodCheck.data.requirements ?? []).map((req) => (
                    <li key={req.id} className="flex gap-3 rounded-xl border px-4 py-3 hairline">
                      <span
                        className={`mt-0.5 flex h-5 w-5 flex-none items-center justify-center rounded-full border text-[11px] ${
                          req.met
                            ? "border-success-500 bg-success-500 text-white"
                            : "border-accent-500 text-accent-600"
                        }`}
                      >
                        {req.met ? "✓" : "!"}
                      </span>
                      <span className="min-w-0">
                        <span className="block text-[13.5px] font-medium text-fg">{req.label}</span>
                        <span className="mt-1 block text-[12.5px] leading-relaxed text-muted">
                          {req.detail}
                        </span>
                      </span>
                    </li>
                  ))}
                </ul>

                {!prodCheck.data.allowed ? (
                  <div className="mt-4 flex flex-wrap gap-2">
                    <Link href={`/apps/${appId}/group`} className="btn-ghost btn-sm">
                      Add operators
                    </Link>
                    {orgId ? (
                      <Link href={`/org/${orgId}/billing`} className="btn-ghost btn-sm">
                        Change plan
                      </Link>
                    ) : null}
                  </div>
                ) : null}

                <Callout
                  title={
                    <>
                      Platform rules, not protocol ones
                      <InfoTip>{prodCheck.data.note}</InfoTip>
                    </>
                  }
                />
              </>
            ) : null}
          </Section>

          <Section title="Identifiers">
            <dl className="space-y-4">
              <div className="min-w-0">
                <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                  App ID
                </dt>
                <dd className="mt-1">
                  <CopyValue value={appId} label="App ID" />
                </dd>
              </div>
              {app.data?.group_address ? (
                <div className="min-w-0">
                  <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                    Group address
                  </dt>
                  <dd className="mt-1">
                    <CopyValue value={app.data.group_address} label="Group address" />
                  </dd>
                </div>
              ) : null}
              <div>
                <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                  Chain
                </dt>
                <dd className="mt-1 text-[13.5px] text-fg">{chainName(app.data?.chain_id)}</dd>
              </div>
              <div>
                <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                  Created
                </dt>
                <dd className="mt-1 text-[13.5px] text-fg">{formatDateTime(app.data?.created_at)}</dd>
              </div>
            </dl>
          </Section>
        </div>
      </div>

      <Modal
        open={archiveOpen}
        onClose={() => setArchiveOpen(false)}
        title="Archive this app"
        description={`Type "${app.data?.name ?? ""}" to confirm.`}
        footer={
          <>
            <button type="button" className="btn-ghost" onClick={() => setArchiveOpen(false)}>
              Cancel
            </button>
            <button
              type="button"
              className="btn-danger"
              disabled={archiving || confirmName !== app.data?.name}
              onClick={() => archive()}
            >
              {archiving ? "Archiving…" : "Archive app"}
            </button>
          </>
        }
      >
        <input
          className="input mt-4"
          value={confirmName}
          onChange={(e) => setConfirmName(e.target.value)}
          placeholder={app.data?.name}
          aria-label="Confirm app name"
        />
      </Modal>
    </>
  );
}
