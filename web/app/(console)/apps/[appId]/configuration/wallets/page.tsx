"use client";

import { use } from "react";
import { Callout, PageHeader, Section, Toggle } from "@/components/ui";
import { SettingsForm } from "@/components/console/SettingsForm";
import { curveLabel } from "@/lib/format";
import type { Curve, EmbeddedWalletSettings } from "@/lib/types";

const CURVES: { id: Curve; description: string }[] = [
  {
    id: "frost_secp256k1",
    description:
      "FROST Schnorr. Verified on-chain by a library rather than a precompile, so it reaches EVM through a smart account.",
  },
  {
    id: "ecdsa_secp256k1",
    description:
      "Threshold ECDSA. Produces signatures that verify under ecrecover — EIP-712, EIP-3009, and anything expecting a normal signature.",
  },
  {
    id: "frost_ed25519",
    description:
      "FROST over Ed25519, for chains that verify Ed25519 natively — Solana, NEAR, Cosmos.",
  },
];

const CHAINS = [
  { id: 1, name: "Ethereum" },
  { id: 8453, name: "Base" },
  { id: 10, name: "Optimism" },
  { id: 42161, name: "Arbitrum One" },
  { id: 137, name: "Polygon" },
  { id: 11155111, name: "Sepolia" },
  { id: 84532, name: "Base Sepolia" },
  { id: 31337, name: "Anvil (local)" },
];

export default function EmbeddedWalletsPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);

  return (
    <>
      <PageHeader
        title="Embedded wallets"
        info="Confirmation prompts are enforced by your app, not by the operators, so a compromised client can skip them. For a constraint the network enforces, use a scoped key."
      />

      <SettingsForm<"embedded_wallets">
        appId={appId}
        section="embedded_wallets"
        defaults={{
          creation: "on_login",
          curves: ["frost_secp256k1"],
          default_curve: "frost_secp256k1",
          prover: "client",
          session_ttl_seconds: 3600,
          require_confirmation_on_transaction: true,
          chain_ids: [],
        }}
      >
        {(value, set) => (
          <>
            <Section
              title="Key creation"
            >
              <div className="grid gap-2 sm:grid-cols-3">
                {(
                  [
                    { id: "on_login", label: "On login", hint: "Every user gets a wallet the first time they sign in." },
                    { id: "on_demand", label: "On demand", hint: "Only when your app first needs one." },
                    { id: "off", label: "Off", hint: "Your backend creates keys explicitly." },
                  ] as const
                ).map((o) => (
                  <button
                    key={o.id}
                    type="button"
                    onClick={() => set({ creation: o.id })}
                    className={`rounded-xl border px-3.5 py-3 text-left transition ${
                      value.creation === o.id
                        ? "border-accent-500 bg-accent-500/[0.08]"
                        : "hover:bg-fg/[0.03] hairline"
                    }`}
                  >
                    <span className="block text-[13.5px] font-medium text-fg">{o.label}</span>
                    <span className="mt-1 block text-[12px] leading-snug text-muted">{o.hint}</span>
                  </button>
                ))}
              </div>
            </Section>

            <Section
              title="Signing schemes"
            >
              <div className="space-y-2.5">
                {CURVES.map((c) => {
                  const on = (value.curves ?? []).includes(c.id);
                  return (
                    <Toggle
                      key={c.id}
                      checked={on}
                      onChange={(v) =>
                        set({
                          curves: v
                            ? [...(value.curves ?? []), c.id]
                            : (value.curves ?? []).filter((x) => x !== c.id),
                        })
                      }
                      label={curveLabel(c.id)}
                    />
                  );
                })}
              </div>

              <div className="mt-5">
                <label className="label" htmlFor="default-curve">
                  Default scheme
                </label>
                <select
                  id="default-curve"
                  className="select"
                  value={value.default_curve ?? "frost_secp256k1"}
                  onChange={(e) => set({ default_curve: e.target.value as Curve })}
                >
                  {(value.curves ?? []).map((c) => (
                    <option key={c} value={c}>
                      {curveLabel(c)}
                    </option>
                  ))}
                </select>
              </div>
            </Section>

            <Section
              title="Proving"
            >
              <div className="grid gap-2 sm:grid-cols-2">
                {(
                  [
                    {
                      id: "client",
                      label: "In the browser",
                      hint: "Strongest. The credential never leaves the user's device. Costs a few seconds at login.",
                    },
                    {
                      id: "server",
                      label: "On your server",
                      hint: "Faster. Suitable if your backend is already trusted with user data.",
                    },
                  ] as const
                ).map((o) => (
                  <button
                    key={o.id}
                    type="button"
                    onClick={() => set({ prover: o.id })}
                    className={`rounded-xl border px-3.5 py-3 text-left transition ${
                      value.prover === o.id
                        ? "border-accent-500 bg-accent-500/[0.08]"
                        : "hover:bg-fg/[0.03] hairline"
                    }`}
                  >
                    <span className="block text-[13.5px] font-medium text-fg">{o.label}</span>
                    <span className="mt-1 block text-[12px] leading-snug text-muted">{o.hint}</span>
                  </button>
                ))}
              </div>
            </Section>

            <Section title="Sessions and confirmation">
              <div className="space-y-5">
                <div className="max-w-xs">
                  <label className="label" htmlFor="ttl">
                    Session lifetime (seconds)
                  </label>
                  <input
                    id="ttl"
                    type="number"
                    min={60}
                    max={86_400}
                    className="input"
                    value={value.session_ttl_seconds ?? 3600}
                    onChange={(e) => set({ session_ttl_seconds: Number(e.target.value) })}
                  />
                </div>

                <Toggle
                  checked={value.require_confirmation_on_sign ?? false}
                  onChange={(v) => set({ require_confirmation_on_sign: v })}
                  label="Confirm every message signature"
                />
                <Toggle
                  checked={value.require_confirmation_on_transaction ?? true}
                  onChange={(v) => set({ require_confirmation_on_transaction: v })}
                  label="Confirm every transaction"
                />

              </div>
            </Section>

            <Section
              title="Chains"
            >
              <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
                {CHAINS.map((c) => {
                  const on = (value.chain_ids ?? []).includes(c.id);
                  return (
                    <button
                      key={c.id}
                      type="button"
                      onClick={() =>
                        set({
                          chain_ids: on
                            ? (value.chain_ids ?? []).filter((x) => x !== c.id)
                            : [...(value.chain_ids ?? []), c.id],
                        })
                      }
                      className={`rounded-xl border px-3.5 py-2.5 text-left text-[13px] transition ${
                        on ? "border-accent-500 bg-accent-500/[0.08] text-fg" : "text-muted hover:bg-fg/[0.03] hairline"
                      }`}
                    >
                      {c.name}
                    </button>
                  );
                })}
              </div>

              {(value.chain_ids ?? []).length > 0 ? (
                <div className="mt-5 max-w-xs">
                  <label className="label" htmlFor="default-chain">
                    Default chain
                  </label>
                  <select
                    id="default-chain"
                    className="select"
                    value={value.default_chain_id ?? (value.chain_ids ?? [])[0]}
                    onChange={(e) => set({ default_chain_id: Number(e.target.value) })}
                  >
                    {(value.chain_ids ?? []).map((id) => (
                      <option key={id} value={id}>
                        {CHAINS.find((c) => c.id === id)?.name ?? `Chain ${id}`}
                      </option>
                    ))}
                  </select>
                </div>
              ) : null}
            </Section>
          </>
        )}
      </SettingsForm>
    </>
  );
}
