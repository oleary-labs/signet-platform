"use client";

import { use } from "react";
import { Callout, PageHeader, Section, Toggle } from "@/components/ui";
import { SettingsForm } from "@/components/console/SettingsForm";
import { uploadFile } from "@/lib/api";
import { useSession } from "@/providers/SessionProvider";
import { useToast } from "@/providers/ToastProvider";
import type { BrandingSettings } from "@/lib/types";

const ACCENTS = ["#e8873c", "#486581", "#3d9f6f", "#c4523b", "#6f42c1", "#0f766e"];

export default function BrandingPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);
  const { activeOrg } = useSession();
  const toast = useToast();

  return (
    <>
      <PageHeader
        title="Branding"
      />

      <SettingsForm<"branding">
        appId={appId}
        section="branding"
        defaults={{
          theme: "system",
          accent_color: "#e8873c",
          corner_radius: "soft",
          show_wallet_ui: true,
        }}
      >
        {(value, set) => (
          <>
            <div className="grid gap-5 lg:grid-cols-3">
              <div className="space-y-5 lg:col-span-2">
                <Section title="Identity">
                  <div className="space-y-5">
                    <div>
                      <label className="label" htmlFor="app-name">
                        Name shown to users
                      </label>
                      <input
                        id="app-name"
                        className="input"
                        placeholder="Acme"
                        value={value.app_name ?? ""}
                        onChange={(e) => set({ app_name: e.target.value })}
                      />
                    </div>

                    <div>
                      <span className="label">Logo</span>
                      <div className="flex items-center gap-4">
                        {value.logo_url ? (
                          // A developer-supplied URL on an arbitrary host, so a
                          // plain <img> rather than next/image.
                          // eslint-disable-next-line @next/next/no-img-element
                          <img
                            src={value.logo_url}
                            alt=""
                            className="h-12 w-12 rounded-xl object-cover"
                          />
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
                              if (!file || !activeOrg) return;
                              try {
                                const url = await uploadFile(file, {
                                  orgId: activeOrg.id,
                                  appId,
                                  scope: "branding",
                                });
                                set({ logo_url: url });
                              } catch (err) {
                                toast.error(
                                  "Upload failed",
                                  err instanceof Error ? err.message : String(err),
                                );
                              }
                            }}
                          />
                        </label>
                        {value.logo_url ? (
                          <button
                            type="button"
                            className="btn-quiet btn-sm"
                            onClick={() => set({ logo_url: "" })}
                          >
                            Remove
                          </button>
                        ) : null}
                      </div>
                    </div>

                    <div>
                      <label className="label" htmlFor="header">
                        Header text
                      </label>
                      <input
                        id="header"
                        className="input"
                        placeholder="Sign in to Acme"
                        value={value.header_text ?? ""}
                        onChange={(e) => set({ header_text: e.target.value })}
                      />
                    </div>

                    <div>
                      <label className="label" htmlFor="message">
                        Login message
                      </label>
                      <textarea
                        id="message"
                        className="textarea"
                        placeholder="A sentence explaining what signing in gives them."
                        value={value.login_message ?? ""}
                        onChange={(e) => set({ login_message: e.target.value })}
                      />
                    </div>
                  </div>
                </Section>

                <Section title="Appearance">
                  <div className="space-y-5">
                    <div>
                      <span className="label">Theme</span>
                      <div className="grid gap-2 sm:grid-cols-3">
                        {(["light", "dark", "system"] as const).map((t) => (
                          <button
                            key={t}
                            type="button"
                            onClick={() => set({ theme: t })}
                            className={`rounded-xl border px-3.5 py-2.5 text-[13.5px] capitalize transition ${
                              value.theme === t
                                ? "border-accent-500 bg-accent-500/[0.08] text-fg"
                                : "text-muted hover:bg-fg/[0.03] hairline"
                            }`}
                          >
                            {t}
                          </button>
                        ))}
                      </div>
                    </div>

                    <div>
                      <span className="label">Accent colour</span>
                      <div className="flex flex-wrap items-center gap-2">
                        {ACCENTS.map((c) => (
                          <button
                            key={c}
                            type="button"
                            aria-label={c}
                            onClick={() => set({ accent_color: c })}
                            className={`h-8 w-8 rounded-lg border-2 transition ${
                              value.accent_color === c ? "border-fg" : "border-transparent"
                            }`}
                            style={{ background: c }}
                          />
                        ))}
                        <input
                          type="color"
                          value={value.accent_color ?? "#e8873c"}
                          onChange={(e) => set({ accent_color: e.target.value })}
                          className="h-8 w-12 cursor-pointer rounded-lg border bg-transparent hairline"
                          aria-label="Custom accent colour"
                        />
                      </div>
                    </div>

                    <div>
                      <span className="label">Corner radius</span>
                      <div className="grid gap-2 sm:grid-cols-3">
                        {(["sharp", "soft", "round"] as const).map((r) => (
                          <button
                            key={r}
                            type="button"
                            onClick={() => set({ corner_radius: r })}
                            className={`border px-3.5 py-2.5 text-[13.5px] capitalize transition ${
                              r === "sharp" ? "rounded-none" : r === "soft" ? "rounded-xl" : "rounded-full"
                            } ${
                              value.corner_radius === r
                                ? "border-accent-500 bg-accent-500/[0.08] text-fg"
                                : "text-muted hover:bg-fg/[0.03] hairline"
                            }`}
                          >
                            {r}
                          </button>
                        ))}
                      </div>
                    </div>

                    <Toggle
                      checked={value.show_wallet_ui ?? true}
                      onChange={(v) => set({ show_wallet_ui: v })}
                      label="Show the built-in wallet screens"
                    />
                  </div>
                </Section>

                <Section title="Legal">
                  <div className="grid gap-5 sm:grid-cols-2">
                    <div>
                      <label className="label" htmlFor="terms">
                        Terms of service URL
                      </label>
                      <input
                        id="terms"
                        className="input"
                        placeholder="https://acme.dev/terms"
                        value={value.terms_url ?? ""}
                        onChange={(e) => set({ terms_url: e.target.value })}
                      />
                    </div>
                    <div>
                      <label className="label" htmlFor="privacy">
                        Privacy policy URL
                      </label>
                      <input
                        id="privacy"
                        className="input"
                        placeholder="https://acme.dev/privacy"
                        value={value.privacy_url ?? ""}
                        onChange={(e) => set({ privacy_url: e.target.value })}
                      />
                    </div>
                  </div>
                </Section>
              </div>

              <div className="space-y-5">
                <Section title="Preview">
                  <LoginPreview branding={value} />
                </Section>
              </div>
            </div>
          </>
        )}
      </SettingsForm>
    </>
  );
}

/** A faithful-enough sketch of the login modal, so the settings above have a
 *  visible consequence rather than being adjusted blind. */
function LoginPreview({ branding }: { branding: BrandingSettings }) {
  const radius =
    branding.corner_radius === "sharp"
      ? "0px"
      : branding.corner_radius === "round"
        ? "999px"
        : "12px";
  const accent = branding.accent_color ?? "#e8873c";
  const dark = branding.theme === "dark";

  return (
    <div
      className="rounded-2xl border p-5 hairline"
      style={{ background: dark ? "#0a1929" : "#ffffff", color: dark ? "#f0f4f8" : "#102a43" }}
    >
      <div className="flex items-center gap-2.5">
        {branding.logo_url ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={branding.logo_url} alt="" className="h-8 w-8 rounded-lg object-cover" />
        ) : (
          <div
            className="flex h-8 w-8 items-center justify-center rounded-lg text-[11px] font-bold text-white"
            style={{ background: accent }}
          >
            {(branding.app_name ?? "A").slice(0, 1).toUpperCase()}
          </div>
        )}
        <p className="text-[14px] font-semibold">
          {branding.header_text || `Sign in to ${branding.app_name || "your app"}`}
        </p>
      </div>

      {branding.login_message ? (
        <p className="mt-3 text-[12.5px] opacity-70">{branding.login_message}</p>
      ) : null}

      <div className="mt-5 space-y-2">
        <div
          className="px-3.5 py-2.5 text-center text-[13px] font-semibold text-white"
          style={{ background: accent, borderRadius: radius }}
        >
          Continue with Google
        </div>
        <div
          className="border px-3.5 py-2.5 text-center text-[13px]"
          style={{ borderRadius: radius, borderColor: dark ? "#334e68" : "#e2dfd9", opacity: 0.85 }}
        >
          Continue with email
        </div>
      </div>

      <p className="mt-4 text-center text-[10.5px] opacity-50">
        Secured by a {"{n}"}-of-{"{m}"} signing group
      </p>
    </div>
  );
}
