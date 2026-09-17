'use client';

import { useId } from 'react';

import { cn } from '@/lib/cn';

export type ProgressBarProps = {
  label: string;
  value: number | null;
  max?: number;
  thresholds?: { value: number; label: string }[];
  valueText?: string;
  tone?: 'primary' | 'success' | 'warning' | 'danger';
};

const fills = { primary: 'bg-primary', success: 'bg-success', warning: 'bg-warning', danger: 'bg-danger' } as const;

/** `value: null` is indeterminate: no aria-valuenow, never a fake zero. Thresholds are ticks with text labels. */
export function ProgressBar({ label, value, max = 100, thresholds = [], valueText, tone = 'primary' }: ProgressBarProps) {
  const labelId = useId();
  const pct = (v: number) => `${Math.min(100, Math.max(0, (v / max) * 100))}%`;
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between gap-2">
        <span id={labelId} className="text-foreground type-small font-semibold">
          {label}
        </span>
        {valueText ? <span className="text-foreground type-data">{valueText}</span> : null}
      </div>
      <div
        role="progressbar"
        aria-labelledby={labelId}
        aria-valuemin={0}
        aria-valuemax={max}
        aria-valuenow={value ?? undefined}
        aria-valuetext={valueText}
        className="relative h-2 rounded-full bg-surface-sunken"
      >
        {value === null ? (
          <div className="absolute inset-y-0 start-0 w-1/3 animate-pulse rounded-full bg-foreground-subtle" />
        ) : (
          <div className={cn('absolute inset-y-0 start-0 rounded-full', fills[tone])} style={{ width: pct(value) }} />
        )}
        {thresholds.map((t) => (
          <div key={t.value} aria-hidden className="absolute -inset-y-1 w-0.5 bg-foreground" style={{ insetInlineStart: pct(t.value) }} />
        ))}
      </div>
      {thresholds.length > 0 ? (
        <ul className="flex flex-wrap gap-x-3 text-foreground-muted type-caption">
          {thresholds.map((t) => (
            <li key={t.value}>{t.label}</li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
