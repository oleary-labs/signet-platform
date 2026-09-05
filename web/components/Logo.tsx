"use client";

/**
 * The Signet mark.
 *
 * No canonical logo existed in any of the Signet repositories, so this one is
 * drawn from the protocol's own idea: a seal whose ring is six separate arcs.
 * The ring is visibly not one piece, and one arc is picked out in the accent —
 * the operator set is chosen, and no single arc is the seal.
 */
export function Logo({ size = 28, className = "" }: { size?: number; className?: string }) {
  const arcs = [
    "M18.223 3.394A12.8 12.8 0 0 1 25.805 7.772",
    "M28.028 11.622A12.8 12.8 0 0 1 28.028 20.378",
    "M25.805 24.228A12.8 12.8 0 0 1 18.223 28.606",
    "M13.777 28.606A12.8 12.8 0 0 1 6.195 24.228",
    "M3.972 20.378A12.8 12.8 0 0 1 3.972 11.622",
    "M6.195 7.772A12.8 12.8 0 0 1 13.777 3.394",
  ];
  return (
    <svg
      viewBox="0 0 32 32"
      width={size}
      height={size}
      className={className}
      role="img"
      aria-label="Signet"
    >
      <title>Signet</title>
      {arcs.map((d, i) => (
        <path
          key={d}
          d={d}
          fill="none"
          stroke={i === 0 ? "#e8873c" : "currentColor"}
          strokeOpacity={i === 0 ? 1 : 0.85}
          strokeWidth={2.3}
          strokeLinecap="round"
        />
      ))}
      <polygon
        points="16.000,10.600 20.677,13.300 20.677,18.700 16.000,21.400 11.323,18.700 11.323,13.300"
        fill="currentColor"
        fillOpacity={0.9}
      />
    </svg>
  );
}

/** Mark plus wordmark, for headers and the marketing pages. */
export function Wordmark({ className = "" }: { className?: string }) {
  return (
    <span className={`inline-flex items-center gap-2.5 ${className}`}>
      <Logo size={26} />
      <span className="text-[17px] font-semibold tracking-tight">Signet</span>
    </span>
  );
}
