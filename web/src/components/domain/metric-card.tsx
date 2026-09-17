'use client';

import { ArrowDown, ArrowUp, Minus } from 'lucide-react';
import type { ReactNode } from 'react';

import { cn } from '@/lib/cn';
import { formatCurrency, formatNumber, unitSymbol, type Unit } from '@/lib/format';

import { Skeleton } from '../ui/skeleton';
import { DataQualityBadge, type DataQuality } from './data-quality-badge';

export type MetricDelta = { value: string; direction: 'up' | 'down' | 'flat'; sentiment: 'good' | 'bad' | 'neutral'; comparisonLabel: string };
export type MetricCardProps = {
  label: string;
  /** Plain decimal string; formatted here with Turkish separators (plan D9). */
  value: string | null;
  unit: Unit;
  delta?: MetricDelta;
  sparkline?: ReactNode;
  quality?: DataQuality;
  /** Caps the shown scale; without it the value keeps the scale the API sent. */
  maxFractionDigits?: number;
  loading?: boolean;
};

const arrows = { up: ArrowUp, down: ArrowDown, flat: Minus } as const;
const tones = { good: 'text-success', bad: 'text-danger', neutral: 'text-foreground-muted' } as const;

/** Colour follows sentiment (a rising cost is bad), the arrow follows direction (07 §6). */
export function MetricCard({ label, value, unit, delta, sparkline, quality, maxFractionDigits, loading = false }: MetricCardProps) {
  const Arrow = delta ? arrows[delta.direction] : null;
  const precision = maxFractionDigits === undefined ? {} : { maxFractionDigits };
  const shown = unit === 'TRY' ? formatCurrency(value, precision) : formatNumber(value, precision);
  const magnitude = delta ? formatNumber(delta.value.replace(/^-/, '')) : '';
  const sign = !delta || /^-?0*(\.0*)?$/.test(delta.value) ? '' : delta.value.startsWith('-') ? '−' : '+';
  return (
    <div className="flex min-w-0 flex-col gap-2 rounded-lg border border-border bg-surface-raised p-4 in-data-[card=shadow]:border-card-edge in-data-[card=shadow]:shadow-md">
      <div className="flex items-start justify-between gap-2">
        <p className="text-foreground-muted type-caption">{label}</p>
        {quality ? <DataQualityBadge quality={quality} /> : null}
      </div>
      {loading ? (
        <>
          <Skeleton className="h-8 w-40" />
          <Skeleton className="h-4 w-28" />
        </>
      ) : (
        <>
          <div className="flex items-end justify-between gap-3">
            <p className="flex min-w-0 flex-wrap items-baseline gap-x-1.5 text-foreground">
              <span className="type-metric [overflow-wrap:anywhere]">{shown}</span>
              {unit !== 'TRY' && value !== null ? <span className="text-foreground-muted type-small">{unitSymbol(unit)}</span> : null}
            </p>
            {sparkline}
          </div>
          {delta && Arrow ? (
            <p className="flex flex-wrap items-center gap-x-1.5 type-small">
              <span data-delta className={cn('inline-flex items-center gap-0.5 font-semibold', tones[delta.sentiment])}>
                <Arrow aria-hidden className="size-3.5" />
                <span className="type-data">{`${sign}%${magnitude}`}</span>
              </span>
              <span className="text-foreground-muted">{delta.comparisonLabel}</span>
            </p>
          ) : null}
        </>
      )}
    </div>
  );
}
