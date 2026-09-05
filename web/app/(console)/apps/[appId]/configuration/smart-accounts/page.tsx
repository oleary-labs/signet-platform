"use client";

import { use } from "react";
import { Callout, PageHeader, Section, Toggle } from "@/components/ui";
import { SettingsForm } from "@/components/console/SettingsForm";
import { useSession } from "@/providers/SessionProvider";

export default function SmartAccountsPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);
  const { network } = useSession();

  return (
    <>
      <PageHeader
        title="Smart accounts"
      />

      <SettingsForm<"smart_accounts">
        appId={appId}
        section="smart_accounts"
        defaults={{
          enabled: true,
          implementation: "signet_kernel",
          paymaster_enabled: false,
          sponsor_first_deployment: true,
        }}
      >
        {(value, set) => (
          <>
            <Section title="Account abstraction">
              <div className="space-y-5">
                <Toggle
                  checked={value.enabled ?? true}
                  onChange={(v) => set({ enabled: v })}
                  label="Give each user a smart account"
                />

                <div>
                  <span className="label">Account implementation</span>
                  <div className="grid gap-2 sm:grid-cols-3">
                    {(
                      [
                        {
                          id: "signet_kernel",
                          label: "Signet · Kernel v3",
                          hint: "ERC-7579 modular account with the FROST validator as its root signer.",
                        },
                        {
                          id: "signet_minimal",
                          label: "Signet · minimal",
                          hint: "The reference SignetAccount. Smaller surface, fewer features.",
                        },
                        {
                          id: "external",
                          label: "Bring your own",
                          hint: "Any ERC-1271 account that accepts your group as a signer.",
                        },
                      ] as const
                    ).map((o) => (
                      <button
                        key={o.id}
                        type="button"
                        onClick={() => set({ implementation: o.id })}
                        className={`rounded-xl border px-3.5 py-3 text-left transition ${
                          value.implementation === o.id
                            ? "border-accent-500 bg-accent-500/[0.08]"
                            : "hover:bg-fg/[0.03] hairline"
                        }`}
                      >
                        <span className="block text-[13.5px] font-medium text-fg">{o.label}</span>
                        <span className="mt-1 block text-[12px] leading-snug text-muted">
                          {o.hint}
                        </span>
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            </Section>

            <Section
              title="Infrastructure"
            >
              <div className="grid gap-5 sm:grid-cols-2">
                <div>
                  <label className="label" htmlFor="entrypoint">
                    EntryPoint
                  </label>
                  <input
                    id="entrypoint"
                    className="input font-mono text-[12.5px]"
                    placeholder={network?.entrypoint_address || "0x…"}
                    value={value.entrypoint ?? ""}
                    onChange={(e) => set({ entrypoint: e.target.value })}
                  />
                </div>
                <div>
                  <label className="label" htmlFor="bundler">
                    Bundler URL
                  </label>
                  <input
                    id="bundler"
                    className="input font-mono text-[12.5px]"
                    placeholder={network?.bundler_url || "https://…"}
                    value={value.bundler_url ?? ""}
                    onChange={(e) => set({ bundler_url: e.target.value })}
                  />
                </div>
              </div>

            </Section>

            <Section title="Gas sponsorship">
              <div className="space-y-5">
                <Toggle
                  checked={value.paymaster_enabled ?? false}
                  onChange={(v) => set({ paymaster_enabled: v })}
                  label="Sponsor gas with a paymaster"
                />
                {value.paymaster_enabled ? (
                  <div>
                    <label className="label" htmlFor="paymaster">
                      Paymaster URL (ERC-7677)
                    </label>
                    <input
                      id="paymaster"
                      className="input font-mono text-[12.5px]"
                      placeholder="https://paymaster.example.com"
                      value={value.paymaster_url ?? ""}
                      onChange={(e) => set({ paymaster_url: e.target.value })}
                    />
                  </div>
                ) : null}
                <Toggle
                  checked={value.sponsor_first_deployment ?? true}
                  onChange={(v) => set({ sponsor_first_deployment: v })}
                  label="Sponsor the first deployment"
                />
              </div>
            </Section>
          </>
        )}
      </SettingsForm>
    </>
  );
}
