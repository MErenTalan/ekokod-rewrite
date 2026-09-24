'use client';

import { useTranslations } from 'next-intl';

import { MetricCard } from '@/components/domain/metric-card';
import type { DataQuality } from '@/components/domain/data-quality-badge';
import type { Unit } from '@/lib/format';

/** A MetricCard whose missing value says "veri yok" rather than a dash that could read as zero. */
export function FigureCard({ label, value, unit, quality }: { label: string; value?: string | null; unit: Unit; quality?: DataQuality }) {
  const t = useTranslations('solarPlants');
  if (value === undefined || value === null) {
    return (
      <div className="flex min-w-0 flex-col gap-2 rounded-lg border border-border bg-surface-raised p-4 in-data-[card=shadow]:border-card-edge in-data-[card=shadow]:shadow-md">
        <p className="text-foreground-muted type-caption">{label}</p>
        <p className="text-foreground-muted type-body-lg">{t('noData')}</p>
      </div>
    );
  }
  return <MetricCard label={label} value={value} unit={unit} quality={quality} maxFractionDigits={2} />;
}
