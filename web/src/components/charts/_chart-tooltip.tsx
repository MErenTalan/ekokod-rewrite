'use client';

import { formatQuantity } from '@/lib/format';

import { RAW, seriesStyle, type ChartSeries, type Datum } from './_theme';

type Entry = { payload?: Record<string, unknown> };
export type ChartTooltipProps = {
  active?: boolean;
  payload?: readonly Entry[];
  label?: string | number;
  series: ChartSeries[];
  formatX?: (x: string | number) => string;
};

/** Every series at the hovered x, formatted from the original decimal strings (07 §5, plan D11). */
export function ChartTooltip({ active, payload, label, series, formatX }: ChartTooltipProps) {
  const point = payload?.[0]?.payload;
  if (!active || !point) return null;
  const raw = (point[RAW] ?? point) as Datum;
  const x = label ?? raw.x;
  return (
    <div className="rounded-md border border-border bg-surface-raised px-3 py-2 text-foreground shadow-md type-small">
      <p className="mb-1 font-semibold">{formatX ? formatX(x) : x}</p>
      <ul className="flex flex-col gap-0.5">
        {series.map((s, i) => (
          <li key={s.key} className="flex items-center justify-between gap-4">
            <span className="flex items-center gap-1.5 text-foreground-muted">
              <svg aria-hidden width="14" height="6">
                <line x1="0" y1="3" x2="14" y2="3" stroke={seriesStyle(s, i).color} strokeWidth="2.5" strokeDasharray={seriesStyle(s, i).dash} />
              </svg>
              {s.label}
            </span>
            <span className="type-data">{formatQuantity(raw[s.key] as string | number | null, s.unit)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
