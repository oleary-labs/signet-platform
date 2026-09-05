"use client";

import { use } from "react";
import {
  Badge,
  Callout,
  EmptyState,
  ErrorNote,
  InfoTip,
  PageHeader,
  Section,
  Skeleton,
  SkeletonRows,
  StatCard,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useAction, useQuery } from "@/lib/hooks";
import { useToast } from "@/providers/ToastProvider";
import { formatDate, formatNumber, formatUSDC } from "@/lib/format";

export default function BillingPage({ params }: { params: Promise<{ orgId: string }> }) {
  const { orgId } = use(params);
  const toast = useToast();

  const billing = useQuery(() => api.billing(orgId), [orgId]);
  const invoices = useQuery(() => api.invoices(orgId), [orgId]);

  const [draft, { pending }] = useAction(async () => {
    const result = await api.draftInvoice(orgId);
    await invoices.refresh();
    toast.info("Draft computed", result.note);
  });

  const account = billing.data?.account;

  return (
    <>
      <PageHeader
        title="Billing"
        info="Payments are not switched on. Usage is metered and priced at the published per-active-wallet rate so you can see the cost; nothing is charged."
      />

      {account && !account.payments_enabled ? (
        <div className="mb-6">
          <Callout
            tone="warn"
            title={
              <>
                Payments are not switched on
                <InfoTip>
                  Everything below is computed from real metered usage at the published rate, so you
                  can see what this would cost. No balance is drawn down and nothing is charged.
                </InfoTip>
              </>
            }
          />
        </div>
      ) : null}

      {billing.error ? <ErrorNote error={billing.error} onRetry={billing.refresh} /> : null}

      {billing.loading ? (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-28 rounded-2xl" />
          ))}
        </div>
      ) : account ? (
        <>
          <div className="mb-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <StatCard
              label="Active wallets this month"
              value={formatNumber(account.estimated_month_maw)}
              hint="Distinct users across every app in this organization."
            />
            <StatCard
              label="Projected cost"
              value={formatUSDC(account.estimated_month_cost_micros)}
              hint={`At ${formatUSDC(account.rate_micros_per_maw)} per active wallet.`}
            />
            <StatCard
              label="Balance"
              value={formatUSDC(account.balance_micros)}
              tone={
                account.balance_micros < account.low_balance_micros && account.payments_enabled
                  ? "warn"
                  : undefined
              }
              hint={account.payments_enabled ? "Drawn down at settlement." : "Not in use yet."}
            />
            <StatCard
              label="Status"
              value={<span className="capitalize">{account.status}</span>}
              hint={account.payments_enabled ? undefined : "Billing is inert until payments launch."}
            />
          </div>

          <div className="grid items-start gap-5 lg:grid-cols-3">
            <Section
              title="Invoices"
              className="lg:col-span-2"
              actions={
                <button type="button" className="btn-ghost btn-sm" disabled={pending} onClick={() => draft()}>
                  {pending ? "Computing…" : "Compute this month"}
                </button>
              }
            >
              {invoices.loading ? (
                <SkeletonRows rows={3} />
              ) : (invoices.data ?? []).length === 0 ? (
                <EmptyState
                  title="No invoices yet"
                />
              ) : (
                <div className="table-wrap">
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Period</th>
                        <th>Active wallets</th>
                        <th>Rate</th>
                        <th>Amount</th>
                        <th>Status</th>
                      </tr>
                    </thead>
                    <tbody>
                      {invoices.data!.map((inv) => (
                        <tr key={inv.id}>
                          <td className="whitespace-nowrap text-[13px] text-fg">
                            {formatDate(inv.period_start)} — {formatDate(inv.period_end)}
                          </td>
                          <td>{formatNumber(inv.active_wallets)}</td>
                          <td className="text-[13px] text-muted">
                            {formatUSDC(inv.rate_micros_per_maw)}
                          </td>
                          <td className="font-medium text-fg">{formatUSDC(inv.amount_micros)}</td>
                          <td>
                            <Badge
                              tone={
                                inv.status === "paid"
                                  ? "success"
                                  : inv.status === "issued"
                                    ? "accent"
                                    : "neutral"
                              }
                            >
                              {inv.status}
                            </Badge>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </Section>

            <div className="space-y-5">
              <Section title="How settlement will work">
                <ul className="space-y-3 text-[13.5px] leading-relaxed text-muted">
                  <li className="flex gap-2.5">
                    <span className="mt-1.5 h-1.5 w-1.5 flex-none rounded-full bg-accent-500" />
                    You keep a USDC balance in a billing contract you top up yourself. No invoice
                    cycle, no card, no churn call.
                  </li>
                  <li className="flex gap-2.5">
                    <span className="mt-1.5 h-1.5 w-1.5 flex-none rounded-full bg-accent-500" />
                    Each period, metered active wallets are settled against that balance.
                  </li>
                  <li className="flex gap-2.5">
                    <span className="mt-1.5 h-1.5 w-1.5 flex-none rounded-full bg-accent-500" />
                    Revenue splits between the operators who served your group and the protocol
                    treasury, at a published rate rather than a negotiated one.
                  </li>
                  <li className="flex gap-2.5">
                    <span className="mt-1.5 h-1.5 w-1.5 flex-none rounded-full bg-accent-500" />
                    A low balance warns first. It never silently stops your users signing in.
                  </li>
                </ul>
              </Section>

            </div>
          </div>
        </>
      ) : null}
    </>
  );
}
