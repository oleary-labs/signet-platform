/** Shared formatting helpers. Kept in one place so an address is truncated the
 *  same way on every screen — a developer scanning for a match should never
 *  have to check whether two screens elided different numbers of characters. */

export function shortAddress(addr: string | null | undefined, lead = 6, tail = 4): string {
  if (!addr) return "—";
  if (addr.length <= lead + tail + 2) return addr;
  return `${addr.slice(0, lead)}…${addr.slice(-tail)}`;
}

export function shortHash(hash: string | null | undefined, lead = 10, tail = 6): string {
  return shortAddress(hash, lead, tail);
}

/** USDC micros → a display string. Amounts are integers on the wire so no
 *  rounding happens before it reaches the screen. */
export function formatUSDC(micros: number): string {
  return (micros / 1_000_000).toLocaleString(undefined, {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: micros % 10_000 === 0 ? 2 : 4,
  });
}

export function formatNumber(n: number | null | undefined): string {
  if (n === null || n === undefined) return "—";
  return n.toLocaleString();
}

export function formatPercent(fraction: number, digits = 1): string {
  return `${(fraction * 100).toFixed(digits)}%`;
}

export function formatDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function relativeTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "—";
  const seconds = Math.round((then - Date.now()) / 1000);
  const abs = Math.abs(seconds);
  const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  if (abs < 60) return rtf.format(Math.round(seconds), "second");
  if (abs < 3600) return rtf.format(Math.round(seconds / 60), "minute");
  if (abs < 86_400) return rtf.format(Math.round(seconds / 3600), "hour");
  if (abs < 2_592_000) return rtf.format(Math.round(seconds / 86_400), "day");
  if (abs < 31_536_000) return rtf.format(Math.round(seconds / 2_592_000), "month");
  return rtf.format(Math.round(seconds / 31_536_000), "year");
}

/** A countdown for a timelock. Returns null once the deadline has passed, so
 *  callers can switch from "waiting" to "ready to execute". */
export function countdown(iso: string | null | undefined): string | null {
  if (!iso) return null;
  const remaining = new Date(iso).getTime() - Date.now();
  if (Number.isNaN(remaining) || remaining <= 0) return null;
  const s = Math.floor(remaining / 1000);
  const d = Math.floor(s / 86_400);
  const h = Math.floor((s % 86_400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m ${s % 60}s`;
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
}

const CURVE_LABELS: Record<string, string> = {
  frost_secp256k1: "FROST · secp256k1",
  frost_ed25519: "FROST · Ed25519",
  ecdsa_secp256k1: "ECDSA · secp256k1",
};

export function curveLabel(curve: string): string {
  return CURVE_LABELS[curve] ?? curve;
}

const CHAIN_NAMES: Record<number, string> = {
  1: "Ethereum",
  10: "Optimism",
  137: "Polygon",
  8453: "Base",
  42161: "Arbitrum One",
  11155111: "Sepolia",
  84532: "Base Sepolia",
  31337: "Anvil (local)",
};

export function chainName(id: number | null | undefined): string {
  if (id === null || id === undefined) return "—";
  return CHAIN_NAMES[id] ?? `Chain ${id}`;
}

const SCOPE_LABELS: Record<string, string> = {
  unscoped: "Unscoped",
  evm_userop: "EVM UserOperation",
  solana_tx: "Solana transaction",
  eip712: "EIP-712 typed data",
};

export function scopeLabel(kind: string): string {
  return SCOPE_LABELS[kind] ?? kind;
}
