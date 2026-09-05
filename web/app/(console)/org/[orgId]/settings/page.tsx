"use client";

import { use, useEffect, useState } from "react";
import { CopyValue, ErrorNote, PageHeader, Section } from "@/components/ui";
import { api, uploadFile } from "@/lib/api";
import { useAction, useQuery } from "@/lib/hooks";
import { useSession } from "@/providers/SessionProvider";
import { useToast } from "@/providers/ToastProvider";
import { formatBytes, formatDate } from "@/lib/format";

export default function OrgSettingsPage({ params }: { params: Promise<{ orgId: string }> }) {
  const { orgId } = use(params);
  const toast = useToast();
  const { refresh } = useSession();

  const org = useQuery(() => api.org(orgId), [orgId]);
  const assets = useQuery(() => api.assets(orgId), [orgId]);

  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [website, setWebsite] = useState("");
  const [logo, setLogo] = useState("");

  useEffect(() => {
    if (!org.data) return;
    setName(org.data.name);
    setEmail(org.data.billing_email ?? "");
    setWebsite(org.data.website_url ?? "");
    setLogo(org.data.logo_url ?? "");
  }, [org.data]);

  const [save, { pending, error }] = useAction(async () => {
    await api.updateOrg(orgId, {
      name,
      billing_email: email,
      website_url: website,
      logo_url: logo,
    });
    await org.refresh();
    await refresh();
    toast.success("Saved");
  });

  const [removeAsset] = useAction(async (id: string) => {
    await api.deleteAsset(orgId, id);
    await assets.refresh();
    toast.success("Asset deleted");
  });

  return (
    <>
      <PageHeader title="Organization settings" />

      <div className="grid items-start gap-5 lg:grid-cols-3">
        <div className="space-y-5 lg:col-span-2">
          <Section title="Details">
            <form
              className="space-y-5"
              onSubmit={(e) => {
                e.preventDefault();
                save();
              }}
            >
              <div>
                <label className="label" htmlFor="org-name">
                  Name
                </label>
                <input id="org-name" className="input" value={name} onChange={(e) => setName(e.target.value)} />
              </div>
              <div>
                <label className="label" htmlFor="org-email">
                  Billing email
                </label>
                <input
                  id="org-email"
                  type="email"
                  className="input"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                />
              </div>
              <div>
                <label className="label" htmlFor="org-site">
                  Website
                </label>
                <input
                  id="org-site"
                  className="input"
                  value={website}
                  onChange={(e) => setWebsite(e.target.value)}
                />
              </div>
              <div>
                <span className="label">Logo</span>
                <div className="flex items-center gap-4">
                  {logo ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img src={logo} alt="" className="h-12 w-12 rounded-xl object-cover" />
                  ) : (
                    <div className="flex h-12 w-12 items-center justify-center rounded-xl border border-dashed text-[11px] text-faint hairline">
                      none
                    </div>
                  )}
                  <label className="btn-ghost btn-sm cursor-pointer">
                    Upload
                    <input
                      type="file"
                      accept="image/png,image/jpeg,image/webp"
                      className="hidden"
                      onChange={async (e) => {
                        const file = e.target.files?.[0];
                        if (!file) return;
                        try {
                          setLogo(await uploadFile(file, { orgId, scope: "org" }));
                          assets.refresh();
                        } catch (err) {
                          toast.error("Upload failed", err instanceof Error ? err.message : String(err));
                        }
                      }}
                    />
                  </label>
                </div>
              </div>

              {error ? <ErrorNote error={error} /> : null}
              <button type="submit" className="btn-accent" disabled={pending}>
                {pending ? "Saving…" : "Save changes"}
              </button>
            </form>
          </Section>

          <Section
            title="Stored assets"
          >
            {(assets.data ?? []).length === 0 ? (
              <p className="text-[13.5px] text-muted">Nothing uploaded yet.</p>
            ) : (
              <div className="table-wrap">
                <table className="table">
                  <thead>
                    <tr>
                      <th>Asset</th>
                      <th>Type</th>
                      <th>Size</th>
                      <th>Uploaded</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {assets.data!.map((a) => (
                      <tr key={a.id}>
                        <td className="min-w-0">
                          <CopyValue value={a.url} display={a.scope} label="Asset URL" />
                        </td>
                        <td className="text-[13px] text-muted">{a.content_type}</td>
                        <td className="text-[13px] text-muted">{formatBytes(a.byte_size)}</td>
                        <td className="whitespace-nowrap text-[13px] text-muted">
                          {formatDate(a.created_at)}
                        </td>
                        <td className="text-right">
                          <button
                            type="button"
                            className="btn-danger btn-sm"
                            onClick={() => removeAsset(a.id)}
                          >
                            Delete
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Section>
        </div>

        <div className="space-y-5">
          <Section title="Identifiers">
            <dl className="space-y-4">
              <div className="min-w-0">
                <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                  Organization ID
                </dt>
                <dd className="mt-1">
                  <CopyValue value={orgId} label="Organization ID" />
                </dd>
              </div>
              <div>
                <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                  Slug
                </dt>
                <dd className="mono mt-1 text-fg">{org.data?.slug ?? "—"}</dd>
              </div>
              <div>
                <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                  Apps
                </dt>
                <dd className="mt-1 text-[13.5px] text-fg">{org.data?.app_count ?? 0}</dd>
              </div>
            </dl>
          </Section>
        </div>
      </div>
    </>
  );
}
