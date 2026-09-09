import type { NodeOperator } from "./types";

/**
 * Nodes and operators are not the same thing, and the difference is the whole
 * product.
 *
 * A row in the directory is one **node** — a signer instance, keyed by its own
 * address. An **operator** is the organisation running it, identified on-chain
 * by `NodeInfo.operator` and surfaced here as `operator_address`. The alpha runs
 * six nodes belonging to two operators, so a screen that counts rows and says
 * "operators" overstates by three times the one number a developer is choosing
 * on: how many independent parties would have to collude.
 *
 * The grouping is derived rather than stored because the chain already answers
 * it. `setOperator` is access-controlled and defaults to the node itself, so
 * `operator_address` is always present and always authoritative — nothing here
 * needs a table to be correct. What it does still lack is a *name*: see
 * `operatorLabel`.
 */
export interface OperatorGroup {
  /** The on-chain operator address, or the node's own when it operates itself. */
  address: string;
  label: string;
  nodes: NodeOperator[];
  onlineCount: number;
  /** True only when every node in the group is verified, so one unreviewed
   *  node cannot inherit a badge from its siblings. */
  verified: boolean;
  category: NodeOperator["category"];
  jurisdiction: string;
  description: string;
  logoUrl: string | null;
  websiteUrl: string | null;
}

/**
 * A display name for an operator, derived from its nodes.
 *
 * Interim. An operator has no record of its own yet, so it has nothing to carry
 * a name — the only text available is each node's, and those are named for the
 * node ("O'Leary Labs — AWS us-west-1"). Taking the segment before the first
 * em-dash recovers the organisation when the nodes are named that way, and the
 * check that every sibling agrees keeps it from inventing one when they are not.
 *
 * The fix is a small operators table keyed by address, holding the name, logo
 * and website — all three of which are properties of the organisation and are
 * currently duplicated across its nodes. Until then this is honest about
 * guessing: it falls back to the node's own name rather than a mangled prefix.
 */
export function operatorLabel(nodes: NodeOperator[]): string {
  const first = nodes[0];
  if (!first) return "Unknown operator";
  if (nodes.length === 1) return first.name;

  const candidate = first.name.split(/\s+[—–-]\s+/)[0]?.trim();
  if (candidate && nodes.every((n) => n.name.startsWith(candidate))) return candidate;
  return first.name;
}

/** Group a flat list of node listings by the operator that runs them. Input
 *  order is preserved, so an already-sorted list stays sorted. */
export function groupByOperator(nodes: NodeOperator[]): OperatorGroup[] {
  const byOperator = new Map<string, NodeOperator[]>();
  for (const node of nodes) {
    // A node with no recorded operator operates itself — which is what the
    // contract's _effectiveOperator does, so matching it keeps the two in step
    // rather than collapsing every such node into one phantom group.
    const key = (node.operator_address || node.address).toLowerCase();
    const existing = byOperator.get(key);
    if (existing) existing.push(node);
    else byOperator.set(key, [node]);
  }

  return [...byOperator.entries()].map(([address, group]) => ({
    address,
    label: operatorLabel(group),
    nodes: group,
    onlineCount: group.filter((n) => n.health?.online).length,
    verified: group.every((n) => n.verified),
    category: group[0].category,
    jurisdiction: group[0].jurisdiction,
    description: group[0].description,
    logoUrl: group.find((n) => n.logo_url)?.logo_url ?? null,
    websiteUrl: group.find((n) => n.website_url)?.website_url ?? null,
  }));
}

/** "6 nodes · 2 operators" — the pair, because either number alone misleads. */
export function describeFleet(nodeCount: number, operatorCount: number): string {
  const nodes = `${nodeCount} node${nodeCount === 1 ? "" : "s"}`;
  const operators = `${operatorCount} operator${operatorCount === 1 ? "" : "s"}`;
  return `${nodes} · ${operators}`;
}
