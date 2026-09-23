'use client';

import { useLocale, useTranslations } from 'next-intl';

import { BarChart } from '@/components/charts/bar-chart';
import { ComboChart } from '@/components/charts/combo-chart';
import { MetricCard } from '@/components/domain/metric-card';
import type { DataQuality } from '@/components/domain/data-quality-badge';
import { Alert } from '@/components/ui/alert';
import { Card } from '@/components/ui/card';
import { StaggerGrid } from '@/components/ui/stagger-grid';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Locale } from '@/i18n/locale';
import type { ReportYearly } from '@/lib/api/types';
import { formatMonth, formatNumber, formatQuantity } from '@/lib/format';

import { formatFigure, formatMoneyList } from './format';
import { deltaIn, metricDelta, moneyIn } from './monthly-report';
import { useValueLabels } from './use-value-labels';

/** A two-column labelled table: the shape most yearly sections take. */
function Rows({ title, rows }: { title: string; rows: [string, string][] }) {
  return (
    <TableContainer label={title}>
      <Table aria-label={title}>
        <TableBody>
          {rows.map(([label, value]) => (
            <TableRow key={label}>
              <TableHead scope="row" className="w-1/2">{label}</TableHead>
              <TableCell numeric>{value}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

/** The yearly report of 01 §7.14, section by section in its order. */
export function YearlyReportView({ payload: y }: { payload: ReportYearly }) {
  const t = useTranslations('reports');
  const locale = useLocale() as Locale;
  const l = useValueLabels();
  const lp = useValueLabels('plants');
  const monthName = (m: number) => formatMonth(`${y.year}-${String(m).padStart(2, '0')}`, locale).split(' ')[0];
  const pct = (v: string | undefined) => (v === undefined ? l.noData : formatQuantity(v, 'percent', { maxFractionDigits: 1 }));
  const kwh = (v: string | undefined) => (v === undefined ? l.noData : formatQuantity(v, 'kWh', { maxFractionDigits: 2 }));
  const tonnes = (v: string | undefined) => (v === undefined ? l.noData : `${formatNumber(v, { maxFractionDigits: 3 })} ${t('yearly.carbon.unit')}`);
  const currency = y.chart_currency ?? 'TRY';
  const quality: DataQuality | undefined = y.partial ? { state: 'incomplete', reason: t('partial') } : undefined;
  const comparisonLabel = t('yearly.cards.comparison');
  const omitted = y.omitted_currencies.length ? t('omittedCurrencies', { currencies: y.omitted_currencies.join(', ') }) : undefined;

  return (
    <div className="flex flex-col gap-6">
      {y.partial ? <Alert tone="info" title={t('partial')} /> : null}

      <Card className="flex flex-col gap-3">
        <h2 className="type-h3">{t('yearly.consumptionTable')}</h2>
        <TableContainer label={t('yearly.consumptionTable')}>
          <Table aria-label={t('yearly.consumptionTable')}>
            <TableHeader>
              <TableRow>
                <TableHead>{t('yearly.columns.month')}</TableHead>
                <TableHead numeric>{t('yearly.columns.consumption')}</TableHead>
                <TableHead numeric>{t('yearly.columns.rooftop')}</TableHead>
                <TableHead numeric>{t('yearly.columns.bill')}</TableHead>
                <TableHead numeric>{t('yearly.columns.reactivePenalty')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {y.months.map((m) => (
                <TableRow key={m.month}>
                  <TableHead scope="row">{monthName(m.month)}</TableHead>
                  <TableCell numeric>{formatFigure(m.consumption, 'kWh', l)}</TableCell>
                  <TableCell numeric>{formatFigure(m.rooftop, 'kWh', l)}</TableCell>
                  <TableCell numeric>{formatMoneyList(m.bill, l).join('; ')}</TableCell>
                  <TableCell numeric>{formatMoneyList(m.reactive_penalty, l).join('; ')}</TableCell>
                </TableRow>
              ))}
              <TableRow className="font-semibold">
                <TableHead scope="row">{t('yearly.total')}</TableHead>
                <TableCell numeric>{formatFigure(y.consumption, 'kWh', l)}</TableCell>
                <TableCell numeric>{formatFigure(y.rooftop, 'kWh', l)}</TableCell>
                <TableCell numeric>{formatMoneyList(y.bill, l).join('; ')}</TableCell>
                <TableCell numeric>{formatMoneyList(y.reactive_penalty, l).join('; ')}</TableCell>
              </TableRow>
              <TableRow>
                <TableHead scope="row">{t('yearly.dailyConsumption')}</TableHead>
                <TableCell numeric>{formatFigure(y.daily_consumption, 'kWh', l)}</TableCell>
                <TableCell numeric />
                <TableCell numeric />
                <TableCell numeric />
              </TableRow>
              <TableRow>
                <TableHead scope="row">{t('yearly.dailyRooftop')}</TableHead>
                <TableCell numeric />
                <TableCell numeric>{formatFigure(y.daily_rooftop, 'kWh', l)}</TableCell>
                <TableCell numeric />
                <TableCell numeric />
              </TableRow>
            </TableBody>
          </Table>
        </TableContainer>
      </Card>

      <Card className="flex flex-col gap-3">
        <h2 className="type-h3">{t('yearly.solarTitle')}</h2>
        <Rows
          title={t('yearly.solarTitle')}
          rows={[
            [t('yearly.solar.total'), formatFigure(y.utility, 'kWh', lp)],
            [t('yearly.solar.target'), kwh(y.target)],
            [t('yearly.solar.achievement'), pct(y.achievement_pct)],
            [t('yearly.solar.daily'), formatFigure(y.daily_utility, 'kWh', lp)],
            ...y.plants.map((p): [string, string] => [
              p.name,
              p.target === undefined
                ? `${formatFigure(p.production, 'kWh', lp)} · ${t('yearly.solar.noTarget')}`
                : `${formatFigure(p.production, 'kWh', lp)} / ${kwh(p.target)} · ${pct(p.achievement_pct)}`,
            ]),
          ]}
        />
      </Card>

      <StaggerGrid className="sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard label={t('yearly.cards.consumption')} value={y.consumption.value ?? null} unit="kWh" quality={quality}
          maxFractionDigits={2} delta={metricDelta(y.consumption_delta.pct, false, comparisonLabel)} />
        <MetricCard label={t('yearly.cards.production')} value={y.production.value ?? null} unit="kWh" quality={quality} maxFractionDigits={2} />
        <MetricCard label={t('yearly.cards.bill')} value={currency === 'TRY' ? moneyIn(y.bill, currency) : null} unit="TRY"
          delta={metricDelta(deltaIn(y.bill_delta, currency), false, comparisonLabel)} />
        <MetricCard label={t('yearly.cards.achievement')} value={y.achievement_pct ?? null} unit="percent" maxFractionDigits={1} />
      </StaggerGrid>

      <div className="grid gap-6 xl:grid-cols-2">
        <ComboChart
          title={t('yearly.historyChart')}
          description={t('yearly.historyChartDescription')}
          xLabel={t('yearly.year')}
          data={y.history.map((h) => ({ x: String(h.year), consumption: h.consumption ?? null, production: h.production ?? null }))}
          bars={[{ key: 'consumption', label: t('yearly.series.consumption'), kind: 'consumption', unit: 'kWh' }]}
          lines={[{ key: 'production', label: t('yearly.series.production'), kind: 'generation', unit: 'kWh' }]}
          empty={{ title: t('monthly.chartEmpty'), description: t('monthly.chartEmptyDescription') }}
        />
        <BarChart
          title={t('yearly.billChart')}
          description={t('yearly.billChartDescription')}
          xLabel={t('yearly.year')}
          data={y.history.map((h) => ({ x: String(h.year), bill: moneyIn(h.bill, currency) }))}
          series={[{ key: 'bill', label: t('yearly.series.bill'), kind: 'cost', unit: 'TRY' }]}
          footnote={omitted}
          empty={{ title: t('monthly.chartEmpty'), description: t('monthly.chartEmptyDescription') }}
        />
      </div>
      <BarChart
        title={t('yearly.targetChart')}
        description={t('yearly.targetChartDescription')}
        xLabel={t('yearly.solar.plant')}
        data={y.plants.map((p) => ({ x: p.name, target: p.target ?? null, actual: p.production.value ?? null }))}
        series={[
          { key: 'target', label: t('yearly.series.target'), kind: 'target', unit: 'kWh' },
          { key: 'actual', label: t('yearly.series.actual'), kind: 'generation', unit: 'kWh' },
        ]}
        empty={{ title: t('monthly.chartEmpty'), description: t('monthly.chartEmptyDescription') }}
      />

      <Card className="flex flex-col gap-3">
        <h2 className="type-h3">{t('yearly.comparisonTitle')}</h2>
        <p className="text-foreground-muted type-small">{t('yearly.comparisonDescription')}</p>
        <Rows
          title={t('yearly.comparisonTitle')}
          rows={[
            [t('yearly.comparison.consumption'), formatFigure(y.consumption, 'kWh', l)],
            [t('yearly.comparison.production'), formatFigure(y.production, 'kWh', l)],
            [t('yearly.comparison.solarShare'), pct(y.solar_share_pct)],
            [t('yearly.comparison.gridShare'), pct(y.grid_share_pct)],
          ]}
        />
      </Card>

      <Card className="flex flex-col gap-3">
        <h2 className="type-h3">{t('yearly.carbonTitle')}</h2>
        {y.carbon ? (
          <>
            <Rows
              title={t('yearly.carbonTitle')}
              rows={[
                [t('yearly.carbon.consumption'), tonnes(y.carbon.consumption_t)],
                [t('yearly.carbon.reduction'), tonnes(y.carbon.reduction_t)],
                [t('yearly.carbon.net'), tonnes(y.carbon.net_t)],
                [t('yearly.carbon.factor'), `${formatNumber(y.carbon.factor)} ${y.carbon.factor_unit}`],
              ]}
            />
            <p className="text-foreground-muted type-small">{t('yearly.carbon.note')}</p>
            {y.carbon.source_year ? (
              <p className="text-foreground-muted type-small">{t('yearly.carbon.sourceYear', { year: String(y.carbon.source_year) })}</p>
            ) : null}
          </>
        ) : (
          <p className="text-foreground-muted type-body">{t('yearly.carbon.unavailable')}</p>
        )}
      </Card>
    </div>
  );
}
