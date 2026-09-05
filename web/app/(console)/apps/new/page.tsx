"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";
import { NodeCard } from "@/components/NodeCard";
import {
  Callout,
  CopyValue,
  EmptyState,
  ErrorNote,
  InfoTip,
  PageHeader,
  Section,
  Skeleton,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useQuery } from "@/lib/hooks";
import { useSession } from "@/providers/SessionProvider";
import { useToast } from "@/providers/ToastProvider";
import { chainName } from "@/lib/format";
import {
  DEPLOY_STAGE_COPY,
  signCreateGroup,
  type DeployStage,
} from "@/lib/deploy";
import { useCanSignOnchain } from "@/lib/onchain";
import { SigningSessionNotice } from "@/components/console/SigningSessionNotice";
import type { NodeOperator } from "@/lib/types";

type Step = "details" | "operators" | "login" | "review" | "deploy";

const STEPS: { id: Step; label: string }[] = [
  { id: "details", label: "Details" },
  { id: "operators", label: "Operators" },
  { id: "login", label: "Login methods" },
  { id: "review", label: "Review" },
  { id: "deploy", label: "Deploy" },
];

const REMOVAL_DELAYS = [
  { seconds: 0, label: "None", hint: "Removals take effect immediately. Only sensible on a devnet." },
  { seconds: 3600, label: "1 hour", hint: "Enough time to notice an unexpected removal." },
  { seconds: 86_400, label: "24 hours", hint: "A sensible default for most applications." },
  { seconds: 604_800, label: "7 days", hint: "Removing an operator becomes a deliberate, visible act." },
];

const KNOWN_ISSUERS = [
  { issuer: "https://accounts.google.com", provider: "google", label: "Google" },
  { issuer: "https://appleid.apple.com", provider: "apple", label: "Apple" },
];

export default function NewAppPage() {
  const router = useRouter();
  const toast = useToast();
  const { activeOrg, network, user, refresh } = useSession();

  const [step, setStep] = useState<Step>("details");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  // Apps are always created in development — production is reached by
  // promotion, once the group and plan requirements are met.
  const environment = "development" as const;
  const [selected, setSelected] = useState<string[]>([]);
  const [threshold, setThreshold] = useState(2);
  const [removalDelay, setRemovalDelay] = useState(86_400);
  const [issuers, setIssuers] = useState<string[]>(["https://accounts.google.com"]);
  const [clientIds, setClientIds] = useState<Record<string, string>>({});

  const [stage, setStage] = useState<DeployStage | null>(null);
  const [deployError, setDeployError] = useState<unknown>(null);
  const [manualAddress, setManualAddress] = useState("");

  const operatorsQuery = useQuery(() => api.nodeOperators(), []);
  const canSign = useCanSignOnchain(user);
  // Development groups run on our operators and our paymaster.
  const sponsored = environment === "development" && (network?.sponsor_group_creation ?? false);

  // A development group runs on O'Leary Labs operators only, so the picker
  // shows exactly the set the platform will actually deploy. Offering the full
  // directory and then refusing at deploy time would waste the developer's
  // choices.
  const restrictedToFirstParty = environment === "development";
  const firstPartyCategory = network?.first_party_category ?? "signet";
  const operators = useMemo(() => {
    const all = operatorsQuery.data ?? [];
    return restrictedToFirstParty
      ? all.filter((o) => o.category === firstPartyCategory)
      : all;
  }, [operatorsQuery.data, restrictedToFirstParty, firstPartyCategory]);

  // Changing environment can invalidate an earlier selection; drop anything the
  // new environment does not permit rather than carrying it silently to a
  // refusal at deploy.
  useEffect(() => {
    const allowed = new Set(operators.map((o) => o.address));
    setSelected((prev) => {
      const next = prev.filter((a) => allowed.has(a));
      return next.length === prev.length ? prev : next;
    });
  }, [operators]);

  const selectedOperators = useMemo(
    () => operators.filter((o) => selected.includes(o.address)),
    [operators, selected],
  );

  const toggle = useCallback(
    (address: string) => {
      setSelected((prev) => {
        const next = prev.includes(address)
          ? prev.filter((a) => a !== address)
          : [...prev, address];
        // Keep the threshold inside the bounds the new set allows, rather than
        // letting the review step present an impossible configuration.
        setThreshold((t) => Math.min(Math.max(t, 1), Math.max(next.length, 1)));
        return next;
      });
    },
    [],
  );

  const deploy = useCallback(
    async (existingGroup?: string) => {
      if (!activeOrg || !network || !user) return;
      setDeployError(null);
      try {
        setStage("building");
        const app = await api.createApp(activeOrg.id, {
          name: name.trim(),
          description,
          environment,
          chain_id: network.chain_id,
        });

        const issuerPayload = issuers.map((i) => ({
          issuer: i,
          clientIds: (clientIds[i] ?? "")
            .split(",")
            .map((c) => c.trim())
            .filter(Boolean),
        }));

        let groupAddress = existingGroup?.trim();
        if (groupAddress) {
          setStage("attaching");
          await api.attachGroup(app.id, groupAddress);
        } else {
          // One path, whatever the environment: the developer's smart wallet
          // calls createGroup, so it is the manager from the first block. The
          // wallet itself is deployed by this same operation if it does not
          // exist yet, which is why a brand-new account can do this at all.
          const userOp = await signCreateGroup({
            network,
            user,
            nodes: selected,
            threshold,
            removalDelaySeconds: removalDelay,
            issuers: issuerPayload,
            authKeys: [],
            sponsored,
            onStage: setStage,
          });

          setStage("submitting");
          const result = await api.deployGroup(app.id, {
            nodes: selected,
            threshold,
            removal_delay_seconds: removalDelay,
            user_op: userOp,
          });
          groupAddress = result.group_address;
        }

        // createGroup wrote these to the contract already. Recording them here
        // adds the console metadata the chain has no room for — a label, a
        // provider icon — and the platform recognises them as unchanged, so no
        // second transaction is sent.
        for (const issuer of issuers) {
          const known = KNOWN_ISSUERS.find((k) => k.issuer === issuer);
          await api.saveIssuer(app.id, {
            issuer,
            client_ids: (clientIds[issuer] ?? "")
              .split(",")
              .map((c) => c.trim())
              .filter(Boolean),
            provider: known?.provider ?? "custom",
            label: known?.label ?? "",
          });
        }

        setStage("done");
        await refresh();
        toast.success(`${app.name} is live`, "Your signing group is attached and operational.");
        router.push(`/apps/${app.id}?created=1`);
      } catch (err) {
        setDeployError(err);
        setStage(null);
      }
    },
    [
      activeOrg, network, user, name, description, environment, selected, threshold,
      removalDelay, issuers, clientIds, refresh, router, toast,
    ],
  );

  const stepIndex = STEPS.findIndex((s) => s.id === step);
  // Development groups are ours to run, and are single-operator by design —
  // requiring two there would contradict the policy on the same screen.
  const minOperators = restrictedToFirstParty ? 1 : 2;
  const canContinue =
    step === "details"
      ? name.trim().length > 0
      : step === "operators"
        ? selected.length >= minOperators &&
          threshold >= 1 &&
          threshold <= selected.length
        : true;

  return (
    <>
      <PageHeader
        eyebrow="New app"
        title="Create a signing group"
        actions={
          <Link href="/apps" className="btn-quiet">
            Cancel
          </Link>
        }
      />

      <ol className="mb-8 flex flex-wrap items-center gap-x-2 gap-y-2">
        {STEPS.map((s, i) => (
          <li key={s.id} className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => i <= stepIndex && setStep(s.id)}
              disabled={i > stepIndex || stage !== null}
              className={`flex items-center gap-2 rounded-lg px-2.5 py-1.5 text-[13px] font-medium transition ${
                i === stepIndex
                  ? "bg-accent-500/14 text-accent-700 dark:text-accent-300"
                  : i < stepIndex
                    ? "text-muted hover:text-fg"
                    : "text-faint"
              }`}
            >
              <span
                className={`flex h-5 w-5 items-center justify-center rounded-full border text-[10px] ${
                  i < stepIndex
                    ? "border-success-500 bg-success-500 text-white"
                    : i === stepIndex
                      ? "border-accent-500 text-accent-600"
                      : "border-fg/15"
                }`}
              >
                {i < stepIndex ? "✓" : i + 1}
              </span>
              {s.label}
            </button>
            {i < STEPS.length - 1 ? <span className="text-faint">→</span> : null}
          </li>
        ))}
      </ol>

      {step === "details" ? (
        <Section title="App">
          <div className="max-w-xl space-y-5">
            <div>
              <label className="label" htmlFor="app-name">App name</label>
              <input
                id="app-name"
                className="input"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Acme Wallet"
                autoFocus
              />
            </div>
            <div>
              <label className="label" htmlFor="app-desc">Description <span className="normal-case text-faint">(optional)</span></label>
              <textarea
                id="app-desc"
                className="textarea"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="What this app does, for your teammates."
              />
            </div>
            <div>
              <span className="label">Chain</span>
              <p className="text-[13.5px] text-fg">{chainName(network?.chain_id)}</p>
            </div>
          </div>
        </Section>
      ) : null}

      {step === "operators" ? (
        <div className="space-y-5">
          {restrictedToFirstParty ? (
            <Callout
              tone="warn"
              title={
                <>
                  Our operators, our gas — a single-operator group
                  <InfoTip>
                    Invite others once it exists. Production needs two operators outside
                    O&rsquo;Leary Labs and a paid plan.
                  </InfoTip>
                </>
              }
            />
          ) : null}

          <Section
            title="Choose your operators"
          >
            {operatorsQuery.loading ? (
              <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                {Array.from({ length: 6 }).map((_, i) => (
                  <Skeleton key={i} className="h-[236px] rounded-2xl" />
                ))}
              </div>
            ) : operators.length === 0 ? (
              <EmptyState
                title="No operators are listed yet"
              />
            ) : (
              <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                {operators.map((o) => (
                  <NodeCard
                    key={o.address}
                    operator={o}
                    selected={selected.includes(o.address)}
                    onToggle={() => toggle(o.address)}
                  />
                ))}
              </div>
            )}
          </Section>

          <Section title="Set your threshold">
            <ThresholdPicker
              total={selected.length}
              threshold={threshold}
              onChange={setThreshold}
              operators={selectedOperators}
              minimum={minOperators}
            />

            <div className="mt-7">
              <span className="label">Removal timelock</span>
              <p className="mb-3 text-[13px] leading-relaxed text-muted">
                How long a queued operator removal waits before it can be executed. The delay is
                what gives you time to notice a removal you did not intend — including one
                initiated by a compromised manager key.
              </p>
              <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
                {REMOVAL_DELAYS.map((d) => (
                  <button
                    key={d.seconds}
                    type="button"
                    onClick={() => setRemovalDelay(d.seconds)}
                    className={`rounded-xl border px-3.5 py-3 text-left transition ${
                      removalDelay === d.seconds
                        ? "border-accent-500 bg-accent-500/[0.08]"
                        : "hover:bg-fg/[0.03] hairline"
                    }`}
                  >
                    <span className="block text-[13.5px] font-medium text-fg">{d.label}</span>
                    <span className="mt-1 block text-[12px] leading-snug text-muted">{d.hint}</span>
                  </button>
                ))}
              </div>
            </div>
          </Section>
        </div>
      ) : null}

      {step === "login" ? (
        <Section
          title="How will your users sign in?"
        >
          <div className="max-w-2xl space-y-3">
            {KNOWN_ISSUERS.map((k) => {
              const on = issuers.includes(k.issuer);
              return (
                <div key={k.issuer} className="rounded-xl border p-4 hairline">
                  <label className="flex cursor-pointer items-start gap-3">
                    <input
                      type="checkbox"
                      checked={on}
                      onChange={(e) =>
                        setIssuers((prev) =>
                          e.target.checked
                            ? [...prev, k.issuer]
                            : prev.filter((i) => i !== k.issuer),
                        )
                      }
                      className="mt-1 accent-[#e8873c]"
                    />
                    <span className="min-w-0">
                      <span className="block text-sm font-medium text-fg">{k.label}</span>
                      <span className="mono block text-faint">{k.issuer}</span>
                    </span>
                  </label>
                  {on ? (
                    <div className="mt-3 pl-7">
                      <label className="label" htmlFor={`cid-${k.provider}`}>
                        Allowed client IDs
                      </label>
                      <input
                        id={`cid-${k.provider}`}
                        className="input"
                        placeholder="123-abc.apps.googleusercontent.com, …"
                        value={clientIds[k.issuer] ?? ""}
                        onChange={(e) =>
                          setClientIds((prev) => ({ ...prev, [k.issuer]: e.target.value }))
                        }
                      />
                    </div>
                  ) : null}
                </div>
              );
            })}
          </div>

        </Section>
      ) : null}

      {step === "review" ? (
        <div className="space-y-5">
          <Section title="Review">
            <dl className="grid gap-x-8 gap-y-5 sm:grid-cols-2">
              <Review label="App">{name || "—"}</Review>
              <Review label="Environment">{environment}</Review>
              <Review label="Chain">{chainName(network?.chain_id)}</Review>
              <Review label="Configuration">
                <span className="mono">
                  {threshold}-of-{selected.length}
                </span>
              </Review>
              <Review label="Removal timelock">
                {REMOVAL_DELAYS.find((d) => d.seconds === removalDelay)?.label ?? `${removalDelay}s`}
              </Review>
              <Review label="Login methods">
                {issuers.length === 0 ? "None — you will add these later" : issuers.join(", ")}
              </Review>
            </dl>

            <div className="mt-6">
              <p className="label">Operators</p>
              <ul className="space-y-2">
                {selectedOperators.map((o) => (
                  <li
                    key={o.address}
                    className="flex items-center justify-between gap-3 rounded-xl border px-3.5 py-2.5 hairline"
                  >
                    <span className="min-w-0">
                      <span className="block truncate text-[13.5px] font-medium text-fg">
                        {o.name}
                      </span>
                      <span className="mono block truncate text-faint">{o.address}</span>
                    </span>
                    {o.is_open === false ? (
                      <span className="flex-none text-[12px] text-accent-600 dark:text-accent-400">
                        Must accept
                      </span>
                    ) : null}
                  </li>
                ))}
              </ul>
            </div>
          </Section>

          {!canSign ? <SigningSessionNotice what="create a group" /> : null}

          {sponsored ? (
            <Callout
              tone="success"
              title={
                <>
                  No wallet prompt, no gas
                  <InfoTip>
                    Your Signet key signs this and our paymaster pays. Your smart wallet calls
                    createGroup, so it is the group&rsquo;s manager from the first block.
                  </InfoTip>
                </>
              }
            />
          ) : (
            <Callout
              title={
                <>
                  Signed by your Signet key — not sponsored
                  <InfoTip>
                    Your smart wallet calls createGroup and becomes the manager. Production groups
                    are not sponsored, so the wallet pays its own gas.
                  </InfoTip>
                </>
              }
            />
          )}

          {selectedOperators.some((o) => o.is_open === false) ? (
            <Callout
              tone="warn"
              title={
                <>
                  Some operators must accept first
                  <InfoTip>
                    Operators that are not open start pending. The group is operational once at
                    least {threshold} are active; the pending list is on the group screen.
                  </InfoTip>
                </>
              }
            />
          ) : null}
        </div>
      ) : null}

      {step === "deploy" ? (
        <Section title="Deploying">
          {stage && stage !== "done" ? (
            <div className="space-y-3">
              <p className="text-[15px] font-medium text-fg">{DEPLOY_STAGE_COPY[stage]}</p>
              <div className="h-1.5 w-full overflow-hidden rounded-full bg-fg/[0.08]">
                <div
                  className="h-full rounded-full bg-accent-500 transition-all duration-500"
                  style={{
                    width: `${((Object.keys(DEPLOY_STAGE_COPY).indexOf(stage) + 1) / Object.keys(DEPLOY_STAGE_COPY).length) * 100}%`,
                  }}
                />
              </div>
              <p className="text-[13px] leading-relaxed text-muted">
                Keep this tab open. Threshold signing waits for a quorum of your operators to
                respond, which can take a few seconds across regions.
              </p>
            </div>
          ) : null}

          {deployError ? (
            <div className="space-y-4">
              <ErrorNote error={deployError} />
              <div className="flex gap-2">
                <button type="button" className="btn-accent" onClick={() => deploy()}>
                  Try again
                </button>
                <button type="button" className="btn-ghost" onClick={() => setStep("review")}>
                  Back to review
                </button>
              </div>

              <div className="border-t pt-5 hairline">
                <p className="text-[13.5px] font-medium text-fg">
                  Already deployed a group elsewhere?
                </p>
                <p className="mt-1 text-[13px] leading-relaxed text-muted">
                  Paste its address and the platform will verify on-chain that you are its manager
                  before linking it.
                </p>
                <div className="mt-3 flex gap-2">
                  <input
                    className="input"
                    placeholder="0x…"
                    value={manualAddress}
                    onChange={(e) => setManualAddress(e.target.value)}
                  />
                  <button
                    type="button"
                    className="btn-ghost flex-none"
                    disabled={!manualAddress.trim()}
                    onClick={() => deploy(manualAddress)}
                  >
                    Link it
                  </button>
                </div>
              </div>
            </div>
          ) : null}
        </Section>
      ) : null}

      {step !== "deploy" ? (
        <div className="mt-6 flex items-center justify-between gap-3">
          <button
            type="button"
            className="btn-ghost"
            disabled={stepIndex === 0}
            onClick={() => setStep(STEPS[Math.max(stepIndex - 1, 0)].id)}
          >
            Back
          </button>
          {step === "review" ? (
            <button
              type="button"
              className="btn-accent"
              disabled={!canSign}
              onClick={() => {
                setStep("deploy");
                deploy();
              }}
            >
              Deploy signing group
            </button>
          ) : (
            <button
              type="button"
              className="btn-accent"
              disabled={!canContinue}
              onClick={() => setStep(STEPS[stepIndex + 1].id)}
            >
              Continue
            </button>
          )}
        </div>
      ) : null}
    </>
  );
}

function Review({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">{label}</dt>
      <dd className="mt-1 text-[13.5px] text-fg">{children}</dd>
    </div>
  );
}

/**
 * The threshold picker.
 *
 * The number that matters is not the threshold on its own but what it buys:
 * how many operators can fail, and how many would have to collude. Both are
 * spelled out as the slider moves, because "3-of-5" alone does not tell a
 * developer whether they picked well.
 */
function ThresholdPicker({
  total,
  threshold,
  onChange,
  operators,
  minimum,
}: {
  total: number;
  threshold: number;
  onChange: (t: number) => void;
  operators: NodeOperator[];
  minimum: number;
}) {
  if (total < minimum) {
    return (
      <p className="text-[13.5px] text-muted">
        Select at least {minimum === 1 ? "one operator" : `${minimum} operators`} to set a
        threshold.
      </p>
    );
  }

  // One operator leaves nothing to choose: the threshold is 1, and the only
  // thing worth saying is what that means.
  if (total === 1) {
    return (
      <div>
        <div className="flex items-baseline gap-3">
          <span className="mono text-[32px] font-semibold leading-none tracking-tight text-fg">
            1-of-1
          </span>
          <span className="text-[13px] text-muted">a single operator</span>
        </div>
        <Callout
          tone="warn"
          title={
            <>
              One signer holds this group
              <InfoTip>
                A development group is free, sponsored, and ours to run — one operator holds the
                whole signing authority, which is exactly why it cannot be promoted to production.
              </InfoTip>
            </>
          }
        />
      </div>
    );
  }

  const tolerated = total - threshold;
  const warning =
    threshold === 1
      ? "A threshold of 1 means any single operator can sign alone. That is the model you came here to avoid."
      : threshold === total
        ? "Requiring every operator means one outage stops all signing. Leave yourself some slack."
        : null;

  return (
    <div>
      <div className="flex items-baseline gap-3">
        <span className="text-[32px] font-semibold leading-none tracking-tight text-fg">
          {threshold}-of-{total}
        </span>
        <span className="text-[13px] text-muted">threshold of {total} operators</span>
      </div>

      <input
        type="range"
        min={1}
        max={total}
        value={threshold}
        onChange={(e) => onChange(Number(e.target.value))}
        className="mt-5 w-full accent-[#e8873c]"
        aria-label="Signing threshold"
      />

      <div className="mt-5 grid gap-3 sm:grid-cols-2">
        <div className="rounded-xl border px-4 py-3 hairline">
          <p className="text-[12px] font-semibold uppercase tracking-[0.08em] text-faint">
            Fault tolerance
          </p>
          <p className="mt-1.5 text-[14px] text-fg">
            {tolerated === 0
              ? "No operator can be offline"
              : `${tolerated} operator${tolerated === 1 ? "" : "s"} can be offline`}
          </p>
        </div>
        <div className="rounded-xl border px-4 py-3 hairline">
          <p className="text-[12px] font-semibold uppercase tracking-[0.08em] text-faint">
            To forge a signature
          </p>
          <p className="mt-1.5 text-[14px] text-fg">
            An attacker must compromise {threshold} of{" "}
            {new Set(operators.map((o) => o.jurisdiction).filter(Boolean)).size || total} distinct
            operators simultaneously
          </p>
        </div>
      </div>

      {warning ? (
        <Callout
          tone="warn"
          title={
            <>
              Worth reconsidering
              <InfoTip>{warning}</InfoTip>
            </>
          }
        />
      ) : null}
    </div>
  );
}
