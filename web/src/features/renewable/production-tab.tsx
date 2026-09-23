'use client';

import { useLocale, useTranslations } from 'next-intl';
import { useMemo } from 'react';

import { LineChart } from '@/components/charts/line-chart';
import type { ChartSeries } from '@/components/charts/_theme';
import type { Granularity } from '@/components/domain/period-filter-bar';
import { Switch } from '@/components/ui/switch';
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import type { Locale } from '@/i18n/locale';
import type { ConsumptionRow } from '@/lib/api/types';
import { formatNumber } from '@/lib/format';
import { formatPeriod } from '@/lib/format-period';

export type ProductionToggles = { active: boolean; inductive: boolean; capacitive: boolean };

const KEYS = ['active', 'inductive', 'capacitive'] as const;
const pick = (row: ConsumptionRow, key: (typeof KEYS)[number]) =>
  key === 'active'
    ? row.active_export
    : key === 'inductive'
      ? row.reactive_inductive_export
      : row.reactive_capacitive_export;

/** The production tab: /generation's export registers as a chart and a table (§7.8). */
export function ProductionTabView({
  rows,
  granularity,
  show,
  onShowChange,
  loading = false,
}: {
  rows: ConsumptionRow[];
  granularity: Granularity;
  show: ProductionToggles;
  onShowChange: (show: ProductionToggles) => void;
  loading?: boolean;
}) {
  const t = useTranslations('renewable');
  const locale = useLocale() as Locale;
  const series = useMemo<ChartSeries[]>(
    () =>
      [
        {
          key: 'active',
          label: t('production.active'),
          kind: 'generation' as const,
          unit: 'kWh' as const,
        },
        {
          key: 'inductive',
          label: t('production.inductive'),
          kind: 'reactiveInductive' as const,
          unit: 'kVArh' as const,
        },
        {
          key: 'capacitive',
          label: t('production.capacitive'),
          kind: 'reactiveCapacitive' as const,
          unit: 'kVArh' as const,
        },
      ].filter((s) => show[s.key as keyof ProductionToggles]),
    [show, t],
  );
  const data = rows.map((row) => ({
    x: formatPeriod(row.period_start, granularity, locale),
    active: row.active_export ?? null,
    inductive: row.reactive_inductive_export ?? null,
    capacitive: row.reactive_capacitive_export ?? null,
  }));
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap gap-4">
        {KEYS.map((key) => (
          <Switch
            key={key}
            label={t(`production.${key}`)}
            checked={show[key]}
            onCheckedChange={(on) => onShowChange({ ...show, [key]: on })}
          />
        ))}
      </div>
      <LineChart
        title={t('production.chartTitle')}
        description={t('production.chartDescription')}
        xLabel={t('production.period')}
        data={data}
        series={series}
        loading={loading}
        empty={{ title: t('production.empty'), description: t('production.emptyHint') }}
      />
      <TableContainer label={t('production.tableLabel')}>
        <Table aria-label={t('production.tableLabel')}>
          <TableHeader>
            <TableRow>
              <TableHead>{t('production.period')}</TableHead>
              {KEYS.map((key) => (
                <TableHead key={key} numeric>
                  {t(`production.${key}`)}
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={row.period_start}>
                <TableCell>{formatPeriod(row.period_start, granularity, locale)}</TableCell>
                {KEYS.map((key) => {
                  const v = pick(row, key);
                  return (
                    <TableCell
                      key={key}
                      numeric
                      className={v ? undefined : 'text-foreground-muted'}
                    >
                      {v ? formatNumber(v, { maxFractionDigits: 2 }) : t('noData')}
                    </TableCell>
                  );
                })}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </div>
  );
}
