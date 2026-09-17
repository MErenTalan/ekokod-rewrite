'use client';

import { useTranslations } from 'next-intl';

import { compareDecimal, fractionToPercent } from '@/lib/decimal';
import { formatQuantity } from '@/lib/format';

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card';
import { ProgressBar } from '../ui/progress-bar';
import { StatusBadge } from '../ui/status-badge';

type Ratio = { ratio: string | null; limit: string };
export type ReactiveStatusCardProps = {
  /** Decimal fractions ('0.25' is 25 %). */
  inductive: Ratio;
  capacitive: Ratio;
  penaltyApplied: boolean | null;
  advisory?: string;
  period: string;
};

function RatioRow({ label, ratio, limit }: Ratio & { label: string }) {
  const t = useTranslations('domain.reactive');
  const limitPct = fractionToPercent(limit);
  const shown = ratio === null ? null : formatQuantity(fractionToPercent(ratio), 'percent');
  const over = ratio !== null && compareDecimal(ratio, limit) > 0;
  const max = Math.max(Number(limitPct) * 1.5, ratio === null ? 0 : Number(fractionToPercent(ratio)));
  return (
    <div className="flex flex-col gap-2">
      <ProgressBar
        label={label}
        value={ratio === null ? null : Number(fractionToPercent(ratio))}
        max={max}
        valueText={shown ?? t('noData')}
        tone={over ? 'danger' : 'success'}
        thresholds={[{ value: Number(limitPct), label: t('limit', { value: formatQuantity(limitPct, 'percent') }) }]}
      />
      <div>
        {ratio === null ? (
          <StatusBadge status="neutral" label={t('noData')} />
        ) : over ? (
          <StatusBadge status="danger" label={t('overLimit')} />
        ) : (
          <StatusBadge status="success" label={t('withinLimit')} />
        )}
      </div>
    </div>
  );
}

/** Inductive and capacitive ratios against their limits, and whether the penalty applied (07 §6). */
export function ReactiveStatusCard({ inductive, capacitive, penaltyApplied, advisory, period }: ReactiveStatusCardProps) {
  const t = useTranslations('domain.reactive');
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <CardDescription>{period}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-5">
        <RatioRow label={t('inductive')} {...inductive} />
        <RatioRow label={t('capacitive')} {...capacitive} />
        {penaltyApplied !== null ? (
          <div>
            <StatusBadge status={penaltyApplied ? 'danger' : 'success'} label={penaltyApplied ? t('penaltyApplied') : t('penaltyNotApplied')} />
          </div>
        ) : null}
        {advisory ? <p className="text-foreground-muted type-small">{advisory}</p> : null}
      </CardContent>
    </Card>
  );
}
