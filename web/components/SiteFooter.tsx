import Link from "next/link";
import { Wordmark } from "./Logo";

const COLUMNS: { title: string; links: { href: string; label: string; external?: boolean }[] }[] = [
  {
    title: "Platform",
    links: [
      { href: "/marketplace", label: "Operator marketplace" },
      { href: "/pricing", label: "Pricing" },
      { href: "/status", label: "Network status" },
      { href: "/style-guide", label: "Design system" },
    ],
  },
  {
    title: "Developers",
    links: [
      { href: "/docs", label: "Documentation" },
      { href: "/docs/quickstart", label: "Quickstart" },
      { href: "/docs/api", label: "Platform API" },
      { href: "/docs/webhooks", label: "Webhooks" },
    ],
  },
  {
    title: "Protocol",
    links: [
      { href: "https://github.com/oleary-labs/signet-protocol", label: "signet-protocol", external: true },
      { href: "https://github.com/oleary-labs/signet-sdk", label: "signet-sdk", external: true },
      { href: "https://github.com/oleary-labs/signet-circuits", label: "signet-circuits", external: true },
      { href: "https://github.com/oleary-labs/signet-wallet", label: "signet-wallet", external: true },
    ],
  },
];

export function SiteFooter() {
  return (
    <footer className="border-t hairline">
      <div className="container-page py-14">
        <div className="grid gap-10 sm:grid-cols-2 lg:grid-cols-4">
          <div>
            <Wordmark className="text-fg" />
            <p className="mt-4 max-w-xs text-[13px] leading-relaxed text-muted">
              Threshold signing governed by on-chain policy, enforced independently by every
              operator, with no operator ever holding a whole key.
            </p>
          </div>
          {COLUMNS.map((col) => (
            <div key={col.title}>
              <p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-faint">
                {col.title}
              </p>
              <ul className="mt-4 space-y-2.5">
                {col.links.map((l) => (
                  <li key={l.href}>
                    {l.external ? (
                      <a
                        href={l.href}
                        target="_blank"
                        rel="noreferrer"
                        className="text-[13.5px] text-muted transition hover:text-fg"
                      >
                        {l.label}
                      </a>
                    ) : (
                      <Link href={l.href} className="text-[13.5px] text-muted transition hover:text-fg">
                        {l.label}
                      </Link>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
        <div className="mt-12 flex flex-wrap items-center justify-between gap-4 border-t pt-6 hairline">
          <p className="text-[12.5px] text-faint">
            © {new Date().getFullYear()} O&rsquo;Leary Labs. Signet is open source.
          </p>
          <p className="text-[12.5px] text-faint">
            The trust hierarchy ends here — not even with us.
          </p>
        </div>
      </div>
    </footer>
  );
}
