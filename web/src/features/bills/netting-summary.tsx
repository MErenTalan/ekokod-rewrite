'use client';

import { useTranslations } from 'next-intl';

import { Badge } from '@/components/ui/badge';
import { Card } from '@/components/ui/card';
import { StatTile } from '@/components/ui/stat-tile';
import { formatNumber } from '@/lib/format';
import type { BillDashboardNetting } from '@/lib/api/types';

export type NettingSummaryViewProps = {
  netting: BillDashboardNetting[];
};

/** The netting summary of 01 §7.10, one block per currency (R253). */
export function NettingSummaryView({ netting }: NettingSummaryViewProps) {
  const t = useTranslations('bills');
  const num = (v: string) => formatNumber(v, { maxFractionDigits: 2 });
  const money = (v: string) => formatNumber(v, { minFractionDigits: 2, maxFractionDigits: 2 });

  return (
    <div className="flex flex-col gap-4">
      {netting.map((n) => (
        <Card key={n.currency} className="flex flex-col gap-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h3 className="type-h3">{t('netting.title')}</h3>
            <div className="flex items-center gap-2">
              <Badge tone="neutral">{n.currency}</Badge>
              <Badge tone={n.net_status === 'net_production' ? 'success' : 'info'}>
                {n.net_status === 'net_production' ? t('netting.netProduction') : t('netting.netConsumption')}
              </Badge>
              <span className="text-foreground-muted type-caption">{t('netting.period')}: {n.period_key}</span>
            </div>
          </div>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <StatTile label={t('netting.consumption')} value={num(n.total_consumption)} unit="kWh" />
            <StatTile label={t('netting.production')} value={num(n.total_production)} unit="kWh" />
            <StatTile label={t('netting.net')} value={num(n.net)} unit="kWh" />
            <StatTile label={t('netting.invoice')} value={money(n.total_invoice)} hint={n.currency} />
          </div>
          {/* An efficiency against zero consumption does not exist; saying so
              is the house style for anything the data cannot answer (R165). */}
          {n.efficiency_pct ? (
            <StatTile label={t('netting.efficiency')} value={num(n.efficiency_pct)} unit="percent" />
          ) : (
            <p className="text-foreground-muted type-caption">{t('netting.efficiencyUnavailable')}</p>
          )}
        </Card>
      ))}
    </div>
  );
}
