'use client';

import { useTranslations } from 'next-intl';
import type { ReactNode } from 'react';

import { compareDecimal, fractionToPercent } from '@/lib/decimal';
import { formatQuantity } from '@/lib/format';

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card';
import { ProgressBar } from '../ui/progress-bar';
import { Skeleton } from '../ui/skeleton';
import { StatusBadge } from '../ui/status-badge';

type Ratio = { ratio: string | null; limit: string };
export type ReactiveStatusCardProps = {
  /** Decimal fractions ('0.25' is 25 %). */
  inductive: Ratio;
  capacitive: Ratio;
  penaltyApplied: boolean | null;
  advisory?: string;
  period: string;
  /** Controls drawn inside the card above the ratios (the dashboard's month and scope pickers). */
  toolbar?: ReactNode;
  /** Extra lines under the advisory, still inside the card. */
  children?: ReactNode;
  loading?: boolean;
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
        missing="empty"
        tone={over ? 'danger' : 'success'}
        thresholds={[{ value: Number(limitPct), label: t('limit', { value: formatQuantity(limitPct, 'percent') }) }]}
      />
      {/* No ratio: the bar's own text already says "Veri yok"; a second badge only repeated it. */}
      {ratio === null ? null : (
        <div>
          <StatusBadge status={over ? 'danger' : 'success'} label={over ? t('overLimit') : t('withinLimit')} />
        </div>
      )}
    </div>
  );
}

/** Inductive and capacitive ratios against their limits, and whether the penalty applied (07 §6). */
export function ReactiveStatusCard({ inductive, capacitive, penaltyApplied, advisory, period, toolbar, children, loading = false }: ReactiveStatusCardProps) {
  const t = useTranslations('domain.reactive');
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <CardDescription>{period}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-5">
        {toolbar}
        {loading ? (
          <Skeleton className="h-40 w-full" />
        ) : (
          <>
            <RatioRow label={t('inductive')} {...inductive} />
            <RatioRow label={t('capacitive')} {...capacitive} />
            {penaltyApplied !== null ? (
              <div>
                <StatusBadge status={penaltyApplied ? 'danger' : 'success'} label={penaltyApplied ? t('penaltyApplied') : t('penaltyNotApplied')} />
              </div>
            ) : null}
            {advisory ? <p className="text-foreground-muted type-small">{advisory}</p> : null}
            {children}
          </>
        )}
      </CardContent>
    </Card>
  );
}
