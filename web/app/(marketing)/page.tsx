import Link from "next/link";
import SnapRoot from "@/components/SnapRoot";
import { Logo } from "@/components/Logo";

/**
 * The landing page.
 *
 * The argument it makes is the one from the protocol's own positioning: the
 * problem is not custody, it is authority — and the only fix is to distribute
 * the authorization decision itself. Everything on this page is downstream of
 * that claim.
 */
export default function LandingPage() {
  return (
    <>
      <SnapRoot />

      {/* ── Hero ───────────────────────────────────────────────── */}
      <section className="snap-section grid-bg">
        <div
          aria-hidden="true"
          className="anim-aurora pointer-events-none absolute left-1/2 top-[-10%] h-[560px] w-[900px] -translate-x-1/2 rounded-full opacity-[0.16] blur-[110px]"
          style={{ background: "radial-gradient(circle, #e8873c 0%, #486581 55%, transparent 72%)" }}
        />
        <div className="container-page relative">
          <div className="mx-auto max-w-3xl text-center">
            <p className="anim-rise inline-flex items-center gap-2 rounded-full border px-3.5 py-1.5 text-[12px] font-medium text-muted hairline">
              <span className="pulse-dot" />
              Live on testnet · founding operator set forming
            </p>

            <h1 className="anim-rise-1 mt-7 text-[clamp(2.4rem,6.2vw,4.4rem)] font-semibold leading-[1.04] tracking-[-0.03em] text-fg">
              Embedded wallets with{" "}
              <span className="text-gradient-accent">no single operator</span>
            </h1>

            <p className="anim-rise-2 mx-auto mt-6 max-w-2xl text-[17px] leading-relaxed text-muted">
              Social login, smart accounts, and threshold signing — with one difference that
              changes the risk profile entirely. You pick the operators. You set the threshold.
              You can change either one, live, without migrating a single key.
            </p>

            <div className="anim-rise-3 mt-9 flex flex-wrap items-center justify-center gap-3">
              <Link href="/login" className="btn-accent px-5 py-3">
                Create a signing group
              </Link>
              <Link href="/marketplace" className="btn-ghost px-5 py-3">
                Browse operators
              </Link>
            </div>

            <p className="anim-rise-4 mt-8 text-[12.5px] text-faint">
              Open source · FROST (RFC 9591) · ZK proof of OAuth · ERC-1271 / ERC-4337
            </p>
          </div>
        </div>
      </section>

      {/* ── The problem ────────────────────────────────────────── */}
      <section className="snap-section">
        <div className="container-page">
          <div className="mx-auto max-w-3xl text-center">
            <p className="fx text-[11px] font-semibold uppercase tracking-[0.14em] text-accent-600 dark:text-accent-400">
              The actual problem
            </p>
            <h2 className="fx fx-d1 mt-4 text-[clamp(1.8rem,4vw,2.8rem)] font-semibold leading-tight tracking-[-0.025em] text-fg">
              It isn&rsquo;t custody. It&rsquo;s authority.
            </h2>
            <p className="fx fx-d2 mx-auto mt-5 max-w-2xl text-[16.5px] leading-relaxed text-muted">
              Where the key lives matters less than who gets to decide it should sign. Concentrate
              that decision in one organization and you have rebuilt the exact attack surface the
              rest of the stack was designed to remove — breaking the math is hard, reaching the
              company that holds the authority is the path nearly every real incident takes.
            </p>
          </div>

          <div className="mt-12 grid gap-4 md:grid-cols-3">
            {[
              {
                title: "Centralized custody",
                body:
                  "One entity decides what gets signed for every customer. A breach, a subpoena, or an acquisition reaches all of it at once.",
              },
              {
                title: "Do it yourself",
                body:
                  "Whoever holds the key has the authority. No threshold, no policy layer, no audit trail. Fine for a demo, dangerous for real money.",
              },
              {
                title: "TEE providers",
                body:
                  "The enclave protects the key material beautifully. It cannot distribute the operator's authority — compromise the operator and a valid signature follows.",
              },
            ].map((c, i) => (
              <div key={c.title} className={`fx fx-d${i + 1} card p-6`}>
                <h3 className="text-[15px] font-semibold text-fg">{c.title}</h3>
                <p className="mt-2.5 text-[14px] leading-relaxed text-muted">{c.body}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── Two layers ─────────────────────────────────────────── */}
      <section className="snap-section bg-fg/[0.02]">
        <div className="container-page">
          <div className="grid items-center gap-12 lg:grid-cols-2">
            <div>
              <p className="fx text-[11px] font-semibold uppercase tracking-[0.14em] text-accent-600 dark:text-accent-400">
                How it works
              </p>
              <h2 className="fx fx-d1 mt-4 text-[clamp(1.8rem,4vw,2.6rem)] font-semibold leading-tight tracking-[-0.025em] text-fg">
                Two layers. Fail-closed.
              </h2>
              <div className="fx fx-d2 mt-6 space-y-6">
                <div>
                  <h3 className="text-[15px] font-semibold text-fg">
                    Layer 1 — the verifier network
                  </h3>
                  <p className="mt-2 text-[14.5px] leading-relaxed text-muted">
                    Every operator in your group independently checks two things before
                    contributing its share: that the principal has rights on this key, and that the
                    signature is bound to the account that will enforce your policy on-chain. They
                    handle proofs, not secrets — a zero-knowledge proof of a valid credential,
                    never the credential itself.
                  </p>
                </div>
                <div>
                  <h3 className="text-[15px] font-semibold text-fg">
                    Layer 2 — the smart account
                  </h3>
                  <p className="mt-2 text-[14.5px] leading-relaxed text-muted">
                    Balances, spending caps, destination allowlists — the invariants that depend on
                    settled state are enforced by the chain itself, atomically, at execution time.
                    ERC-1271 admits multi-party signers natively, so Layer 1 drops in as the
                    account&rsquo;s signer with no custom integration.
                  </p>
                </div>
                <p className="text-[14.5px] leading-relaxed text-fg">
                  Both must approve. Either can veto. Neither can override the other.
                </p>
              </div>
            </div>

            <div className="fx-scale fx-d2">
              <ThresholdDiagram />
            </div>
          </div>
        </div>
      </section>

      {/* ── Against the incumbent ──────────────────────────────── */}
      <section className="snap-section">
        <div className="container-page">
          <div className="mx-auto max-w-2xl text-center">
            <p className="fx text-[11px] font-semibold uppercase tracking-[0.14em] text-accent-600 dark:text-accent-400">
              Feature parity, different foundation
            </p>
            <h2 className="fx fx-d1 mt-4 text-[clamp(1.8rem,4vw,2.6rem)] font-semibold leading-tight tracking-[-0.025em] text-fg">
              Everything you expect. Nothing you have to trust.
            </h2>
            <p className="fx fx-d2 mx-auto mt-5 text-[16px] leading-relaxed text-muted">
              Social login, embedded wallets, smart accounts, session signers, policies, webhooks,
              per-active-wallet pricing. The developer experience is the one the market already
              settled on — what changes is who can authorize on your behalf.
            </p>
          </div>

          <div className="mx-auto mt-12 max-w-3xl overflow-hidden rounded-2xl border hairline">
            <table className="table">
              <thead>
                <tr>
                  <th>Property</th>
                  <th className="text-center">Centralized providers</th>
                  <th className="text-center">Signet</th>
                </tr>
              </thead>
              <tbody>
                {[
                  ["Social login and embedded wallets", true, true],
                  ["Smart accounts (ERC-4337)", true, true],
                  ["Session signers for agents", true, true],
                  ["You choose the operator set", false, true],
                  ["You set the signing threshold", false, true],
                  ["Swap operators with no downtime or migration", false, true],
                  ["Run some operators yourself", false, true],
                  ["Non-custody you can verify, not just read", false, true],
                  ["Open source, auditable end to end", false, true],
                ].map(([label, them, us], i) => (
                  <tr key={String(label)} className={`fx fx-d${Math.min(i + 1, 4)}`}>
                    <td className="text-fg">{label}</td>
                    <td className="text-center">
                      {them ? (
                        <span className="text-success-600">✓</span>
                      ) : (
                        <span className="text-faint">—</span>
                      )}
                    </td>
                    <td className="text-center font-semibold text-accent-600 dark:text-accent-400">
                      {us ? "✓" : "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </section>

      {/* ── Close ──────────────────────────────────────────────── */}
      <section className="snap-section snap-end bg-primary-950 text-primary-50 dark:bg-black/40">
        <div className="container-page relative">
          <div
            aria-hidden="true"
            className="anim-aurora pointer-events-none absolute left-1/2 top-1/2 h-[440px] w-[720px] -translate-x-1/2 -translate-y-1/2 rounded-full opacity-25 blur-[110px]"
            style={{ background: "radial-gradient(circle, #e8873c 0%, transparent 70%)" }}
          />
          <div className="relative mx-auto max-w-2xl text-center">
            <Logo size={52} className="mx-auto text-primary-100" />
            <h2 className="fx mt-8 text-[clamp(1.9rem,4.4vw,3rem)] font-semibold leading-tight tracking-[-0.03em]">
              Your wallet infrastructure should be as decentralized as the chain it sits on.
            </h2>
            <p className="fx fx-d1 mx-auto mt-6 max-w-xl text-[16px] leading-relaxed text-primary-300">
              Create a group in a few minutes: choose your operators, set your threshold, and walk
              away with a working application key.
            </p>
            <div className="fx fx-d2 mt-9 flex flex-wrap items-center justify-center gap-3">
              <Link href="/login" className="btn-accent px-5 py-3">
                Create a signing group
              </Link>
              <Link
                href="/docs"
                className="btn px-5 py-3 border border-primary-600 text-primary-100 hover:border-primary-400"
              >
                Read the docs
              </Link>
            </div>
          </div>
        </div>
      </section>
    </>
  );
}

/**
 * A 3-of-5 group, drawn rather than described.
 *
 * The point the picture has to make is that the shares are separate and the
 * signature only exists when a threshold of them combine — so the operators
 * are drawn as distinct nodes on a ring with the quorum highlighted, not as a
 * single box labelled "MPC".
 */
function ThresholdDiagram() {
  const nodes = Array.from({ length: 5 }, (_, i) => {
    const angle = (-90 + i * 72) * (Math.PI / 180);
    return {
      i,
      x: 160 + 108 * Math.cos(angle),
      y: 150 + 108 * Math.sin(angle),
      inQuorum: i < 3,
    };
  });

  return (
    <div className="card overflow-hidden p-6">
      <div className="mb-5 flex items-center justify-between">
        <p className="text-[13px] font-semibold text-fg">A 3-of-5 group</p>
        <p className="mono text-faint">threshold 3 · parties 5</p>
      </div>
      <svg viewBox="0 0 320 300" className="w-full" role="img" aria-label="A 3-of-5 threshold signing group">
        <title>Three of five operators combine partial signatures into one signature</title>
        {nodes.map((n) => (
          <line
            key={`l-${n.i}`}
            x1={n.x}
            y1={n.y}
            x2={160}
            y2={150}
            stroke={n.inQuorum ? "#e8873c" : "currentColor"}
            strokeOpacity={n.inQuorum ? 0.55 : 0.14}
            strokeWidth={n.inQuorum ? 1.6 : 1.1}
            strokeDasharray={n.inQuorum ? undefined : "3 4"}
          />
        ))}

        <circle cx="160" cy="150" r="34" fill="currentColor" fillOpacity={0.06} />
        <circle cx="160" cy="150" r="34" fill="none" stroke="#e8873c" strokeOpacity={0.7} strokeWidth="1.6" />
        <text x="160" y="146" textAnchor="middle" className="fill-current" fontSize="11" fontWeight="600">
          one
        </text>
        <text x="160" y="160" textAnchor="middle" className="fill-current" fontSize="11" fontWeight="600">
          signature
        </text>

        {nodes.map((n) => (
          <g key={`n-${n.i}`}>
            <circle
              cx={n.x}
              cy={n.y}
              r="21"
              fill={n.inQuorum ? "#e8873c" : "currentColor"}
              fillOpacity={n.inQuorum ? 0.16 : 0.05}
              stroke={n.inQuorum ? "#e8873c" : "currentColor"}
              strokeOpacity={n.inQuorum ? 0.85 : 0.22}
              strokeWidth="1.5"
            />
            <text
              x={n.x}
              y={n.y + 4}
              textAnchor="middle"
              fontSize="10.5"
              fontWeight="600"
              className="fill-current"
              opacity={n.inQuorum ? 1 : 0.45}
            >
              {n.inQuorum ? "share" : "idle"}
            </text>
          </g>
        ))}
      </svg>
      <p className="mt-4 text-[13px] leading-relaxed text-muted">
        Any three of the five produce the signature. Two can be offline, or actively hostile, and
        nothing changes — and no operator, at any point, holds a whole key.
      </p>
    </div>
  );
}
