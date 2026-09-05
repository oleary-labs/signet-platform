import Link from "next/link";
import type { Metadata } from "next";
import { Callout, PageHeader } from "@/components/ui";

export const metadata: Metadata = {
  title: "Pricing",
  description:
    "Per monthly active wallet, the same unit the rest of the market prices on — so the comparison is direct.",
};

const TIERS = [
  {
    name: "Developer",
    price: "Free",
    unit: "up to 1,000 monthly active wallets",
    description: "Everything works. No feature gates, no trial clock.",
    features: [
      "Unlimited apps and signing groups",
      "Every login method and curve",
      "Session signers and scoped keys",
      "Webhooks, analytics, and audit log",
      "Community support",
    ],
    cta: { href: "/login", label: "Start building" },
  },
  {
    name: "Team",
    price: "$0.05",
    unit: "per monthly active wallet, after the first 1,000",
    description:
      "The rate the network's economics are built on, paid in USDC against your group's balance.",
    features: [
      "Everything in Developer",
      "Organization members and roles",
      "Your own operator selection and threshold",
      "Reshare on demand, no key migration",
      "Email support",
    ],
    cta: { href: "/login", label: "Create an organization" },
    featured: true,
  },
  {
    name: "Enterprise",
    price: "Talk to us",
    unit: "hybrid and self-hosted operator sets",
    description:
      "Run a majority of your own operators, keep a minority external, and prove to compliance that no single vendor holds your keys.",
    features: [
      "Everything in Team",
      "Run your own nodes alongside external ones",
      "Contractual SLAs with named operators",
      "Compliance and audit documentation",
      "Named security contact",
    ],
    cta: { href: "mailto:operators@olearylabs.com", label: "Contact us" },
  },
];

export default function PricingPage() {
  return (
    <div className="container-page py-14">
      <PageHeader
        eyebrow="Pricing"
        title="Priced per active wallet, like the rest of the market"
      />

      <div className="grid gap-5 lg:grid-cols-3">
        {TIERS.map((tier, i) => (
          <div
            key={tier.name}
            className={`fx fx-d${i + 1} card flex flex-col p-6 ${
              tier.featured ? "border-accent-500/50 ring-1 ring-accent-500/20" : ""
            }`}
          >
            {tier.featured ? (
              <p className="mb-3 text-[11px] font-semibold uppercase tracking-[0.12em] text-accent-600 dark:text-accent-400">
                Most common
              </p>
            ) : null}
            <h2 className="text-[17px] font-semibold text-fg">{tier.name}</h2>
            <p className="mt-4 text-[34px] font-semibold leading-none tracking-tight text-fg">
              {tier.price}
            </p>
            <p className="mt-2 text-[13px] text-muted">{tier.unit}</p>
            <p className="mt-4 text-[13.5px] leading-relaxed text-muted">{tier.description}</p>

            <ul className="mt-6 flex-1 space-y-2.5">
              {tier.features.map((f) => (
                <li key={f} className="flex gap-2.5 text-[13.5px] text-muted">
                  <span className="mt-[7px] h-1.5 w-1.5 flex-none rounded-full bg-accent-500" />
                  {f}
                </li>
              ))}
            </ul>

            <Link
              href={tier.cta.href}
              className={`mt-7 ${tier.featured ? "btn-accent" : "btn-ghost"} w-full`}
            >
              {tier.cta.label}
            </Link>
          </div>
        ))}
      </div>

      <div className="mt-10 grid gap-5 lg:grid-cols-2">
        <Callout tone="warn" title="Payments are not switched on yet">
          Usage is metered and the console shows what it would cost at the published rate, but
          nothing is being charged and no balance is being drawn down. When settlement goes live,
          you top up a USDC balance against your group&rsquo;s billing contract — there is no
          invoice cycle to negotiate and no card to churn.
        </Callout>

        <Callout title="What an active wallet means">
          A unique authenticated identity that performs at least one key generation or signing
          operation in the billing period. The nodes meter identity hashes, not identities — the
          platform can count your active wallets without ever learning who your users are.
        </Callout>
      </div>

      <div className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight text-fg">Where the money goes</h2>
        <p className="mt-2 max-w-prose text-sm leading-relaxed text-muted">
          Revenue is split between the operators who actually served your group and the protocol
          treasury. Operators are compensated as service providers with modelable unit economics,
          which is what makes recruiting a credible operator set a business decision rather than a
          speculative one. The split is a governance parameter, published rather than negotiated
          per customer.
        </p>
      </div>
    </div>
  );
}
