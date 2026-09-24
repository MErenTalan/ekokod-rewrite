'use client';

import { useTranslations } from 'next-intl';

import { StaggerGrid } from '@/components/ui/stagger-grid';
import type { RenewableOverview } from '@/lib/api/types';

import { FigureCard } from '../solar-plants/figure-card';

/** §7.8's summary cards: the range's generation registers; missing is "veri yok". */
export function SummaryCardsView({ data }: { data?: RenewableOverview }) {
  const t = useTranslations('renewable.summary');
  return (
    <StaggerGrid className="sm:grid-cols-2 xl:grid-cols-4">
      <FigureCard label={t('active')} value={data?.active_generation_kwh} unit="kWh" />
      <FigureCard label={t('inductive')} value={data?.inductive_generation_kvarh} unit="kVArh" />
      <FigureCard label={t('capacitive')} value={data?.capacitive_generation_kvarh} unit="kVArh" />
      <FigureCard label={t('average')} value={data?.average_generation_kwh} unit="kWh" />
    </StaggerGrid>
  );
}
