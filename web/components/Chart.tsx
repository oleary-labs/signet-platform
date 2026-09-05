"use client";

import { useId, useMemo, useState } from "react";

export interface Series {
  key: string;
  label: string;
  color: string;
  values: number[];
}

/**
 * A small multi-series area chart, drawn by hand.
 *
 * There is no chart library here on purpose: the console draws exactly two
 * shapes, and a dependency that ships its own rendering model would be larger
 * than the code it replaced. Colours come from the Signet palette so the chart
 * reads as part of the product rather than a widget dropped into it.
 */
export function AreaChart({
  labels,
  series,
  height = 220,
  valueFormat = (v: number) => v.toLocaleString(),
}: {
  labels: string[];
  series: Series[];
  height?: number;
  valueFormat?: (v: number) => string;
}) {
  const id = useId();
  const [hover, setHover] = useState<number | null>(null);

  const width = 800;
  const padding = { top: 12, right: 8, bottom: 26, left: 42 };
  const plotW = width - padding.left - padding.right;
  const plotH = height - padding.top - padding.bottom;

  const max = useMemo(() => {
    const m = Math.max(1, ...series.flatMap((s) => s.values));
    // Round the axis up to something readable rather than to the exact peak,
    // so the top gridline is a number a person would say out loud.
    const magnitude = Math.pow(10, Math.floor(Math.log10(m)));
    return Math.ceil(m / magnitude) * magnitude;
  }, [series]);

  const n = labels.length;
  const x = (i: number) => padding.left + (n <= 1 ? plotW / 2 : (i / (n - 1)) * plotW);
  const y = (v: number) => padding.top + plotH - (v / max) * plotH;

  if (n === 0) {
    return (
      <div className="flex h-[220px] items-center justify-center rounded-xl border border-dashed text-[13px] text-muted hairline">
        No activity in this range yet.
      </div>
    );
  }

  const gridlines = [0, 0.25, 0.5, 0.75, 1];

  return (
    <div className="relative">
      <svg
        viewBox={`0 0 ${width} ${height}`}
        className="w-full"
        role="img"
        aria-label={`${series.map((s) => s.label).join(", ")} over ${n} days`}
        onMouseLeave={() => setHover(null)}
      >
        <defs>
          {series.map((s) => (
            <linearGradient key={s.key} id={`${id}-${s.key}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={s.color} stopOpacity="0.26" />
              <stop offset="100%" stopColor={s.color} stopOpacity="0" />
            </linearGradient>
          ))}
        </defs>

        {gridlines.map((g) => (
          <g key={g}>
            <line
              x1={padding.left}
              x2={width - padding.right}
              y1={padding.top + plotH * (1 - g)}
              y2={padding.top + plotH * (1 - g)}
              stroke="currentColor"
              strokeOpacity="0.08"
              strokeWidth="1"
            />
            <text
              x={padding.left - 8}
              y={padding.top + plotH * (1 - g) + 3.5}
              textAnchor="end"
              fontSize="10"
              className="fill-current"
              opacity="0.45"
            >
              {valueFormat(Math.round(max * g))}
            </text>
          </g>
        ))}

        {series.map((s) => {
          const line = s.values.map((v, i) => `${i === 0 ? "M" : "L"}${x(i)} ${y(v)}`).join(" ");
          const area = `${line} L${x(n - 1)} ${padding.top + plotH} L${x(0)} ${padding.top + plotH} Z`;
          return (
            <g key={s.key}>
              <path d={area} fill={`url(#${id}-${s.key})`} />
              <path d={line} fill="none" stroke={s.color} strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" />
            </g>
          );
        })}

        {hover !== null ? (
          <g>
            <line
              x1={x(hover)}
              x2={x(hover)}
              y1={padding.top}
              y2={padding.top + plotH}
              stroke="currentColor"
              strokeOpacity="0.22"
              strokeDasharray="3 3"
            />
            {series.map((s) => (
              <circle key={s.key} cx={x(hover)} cy={y(s.values[hover] ?? 0)} r="3.5" fill={s.color} />
            ))}
          </g>
        ) : null}

        {/* Invisible hit targets, so hovering anywhere in a column works. */}
        {labels.map((_, i) => (
          <rect
            key={i}
            x={x(i) - plotW / Math.max(n - 1, 1) / 2}
            y={padding.top}
            width={plotW / Math.max(n - 1, 1)}
            height={plotH}
            fill="transparent"
            onMouseEnter={() => setHover(i)}
          />
        ))}

        {labels.map((l, i) =>
          i % Math.ceil(n / 7) === 0 || i === n - 1 ? (
            <text
              key={l}
              x={x(i)}
              y={height - 6}
              textAnchor={i === 0 ? "start" : i === n - 1 ? "end" : "middle"}
              fontSize="10"
              className="fill-current"
              opacity="0.45"
            >
              {l.slice(5)}
            </text>
          ) : null,
        )}
      </svg>

      <div className="mt-3 flex flex-wrap items-center gap-4">
        {series.map((s) => (
          <span key={s.key} className="inline-flex items-center gap-2 text-[12.5px] text-muted">
            <span className="h-2 w-2 rounded-full" style={{ background: s.color }} />
            {s.label}
            {hover !== null ? (
              <span className="font-medium text-fg">{valueFormat(s.values[hover] ?? 0)}</span>
            ) : null}
          </span>
        ))}
        {hover !== null ? (
          <span className="ml-auto text-[12.5px] text-faint">{labels[hover]}</span>
        ) : null}
      </div>
    </div>
  );
}

/** Chart colours, drawn from the Signet palette rather than a default ramp. */
export const CHART_COLORS = {
  accent: "#e8873c",
  primary: "#486581",
  success: "#3d9f6f",
  error: "#c4523b",
  muted: "#9fb3c8",
} as const;
