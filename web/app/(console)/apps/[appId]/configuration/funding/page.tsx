"use client";

import { use } from "react";
import {
  Callout,
  InfoTip,
  PageHeader,
  Section,
  Toggle,
} from "@/components/ui";
import { SettingsForm } from "@/components/console/SettingsForm";

const PROVIDERS = [
  { id: "moonpay", label: "MoonPay", description: "Card and bank on-ramp, broad country coverage." },
  { id: "coinbase", label: "Coinbase Onramp", description: "On-ramp for users with a Coinbase account." },
  { id: "transak", label: "Transak", description: "Card on-ramp with a wide asset list." },
  { id: "bridge", label: "Cross-chain bridge", description: "Move assets a user already holds to the chain your app runs on." },
];

export default function FundingPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);

  return (
    <>
      <PageHeader
        title="Funding"
      />

      <div className="mb-5">
        <Callout
          tone="warn"
          title={
            <>
              Not implemented by the protocol
              <InfoTip>
                On-ramps sit outside Signet. These settings record which provider your app offers;
                wiring it up is work in your own client.
              </InfoTip>
            </>
          }
        />
      </div>

      <SettingsForm<"funding">
        appId={appId}
        section="funding"
        defaults={{ enabled: false, providers: [], default_asset: "USDC" }}
      >
        {(value, set) => (
          <>
            <Section title="Providers">
              <div className="space-y-5">
                <Toggle
                  checked={value.enabled ?? false}
                  onChange={(v) => set({ enabled: v })}
                  label="Offer funding in your app"
                />

                <div className="space-y-2.5">
                  {PROVIDERS.map((p) => {
                    const on = (value.providers ?? []).includes(p.id);
                    return (
                      <Toggle
                        key={p.id}
                        checked={on}
                        disabled={!value.enabled}
                        onChange={(v) =>
                          set({
                            providers: v
                              ? [...(value.providers ?? []), p.id]
                              : (value.providers ?? []).filter((x) => x !== p.id),
                          })
                        }
                        label={p.label}
                      />
                    );
                  })}
                </div>

                <div className="max-w-xs">
                  <label className="label" htmlFor="asset">
                    Default asset
                  </label>
                  <select
                    id="asset"
                    className="select"
                    value={value.default_asset ?? "USDC"}
                    onChange={(e) => set({ default_asset: e.target.value })}
                    disabled={!value.enabled}
                  >
                    {["USDC", "USDT", "ETH", "SOL"].map((a) => (
                      <option key={a} value={a}>
                        {a}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
            </Section>
          </>
        )}
      </SettingsForm>
    </>
  );
}
