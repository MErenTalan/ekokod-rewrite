'use client';

import { useTranslations } from 'next-intl';

import { MetricCard } from '@/components/domain/metric-card';
import { StaggerGrid } from '@/components/ui/stagger-grid';
import type { ConsumptionSummary } from '@/lib/api/types';

export type SummaryCardsViewProps = { summary: ConsumptionSummary | null; loading?: boolean };

/** The four summary figures of 01 §7.3, marked when any period is suspect. */
export function SummaryCardsView({ summary, loading = false }: SummaryCardsViewProps) {
  const t = useTranslations('consumption.summary');
  const quality =
    summary && summary.suspect_rows > 0
      ? ({ state: 'suspect', reason: t('suspect', { count: summary.suspect_rows }) } as const)
      : undefined;
  const cards = [
    { label: t('active'), value: summary?.totals.active_import ?? null, unit: 'kWh' as const },
    { label: t('inductive'), value: summary?.totals.reactive_inductive_import ?? null, unit: 'kVArh' as const },
    { label: t('capacitive'), value: summary?.totals.reactive_capacitive_import ?? null, unit: 'kVArh' as const },
    { label: t('average'), value: summary?.averages.active_import ?? null, unit: 'kWh' as const },
  ];
  return (
    <StaggerGrid className="sm:grid-cols-2 xl:grid-cols-4">
      {cards.map((card) => (
        <MetricCard key={card.label} label={card.label} value={card.value} unit={card.unit} quality={quality} loading={loading} />
      ))}
    </StaggerGrid>
  );
}
