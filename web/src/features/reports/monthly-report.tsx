'use client';

import { useLocale, useTranslations } from 'next-intl';

import { BarChart } from '@/components/charts/bar-chart';
import { MetricCard, type MetricDelta } from '@/components/domain/metric-card';
import type { DataQuality } from '@/components/domain/data-quality-badge';
import { Alert } from '@/components/ui/alert';
import { Card } from '@/components/ui/card';
import { StaggerGrid } from '@/components/ui/stagger-grid';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableRow } from '@/components/ui/table';
import type { Locale } from '@/i18n/locale';
import type { ReportCurrencyDelta, ReportDelta, ReportMoney, ReportMonthPoint, ReportMonthly } from '@/lib/api/types';
import { formatMonth, formatNumber } from '@/lib/format';

import { deltaDirection, formatFigure, formatMoneyList, formatRange } from './format';
import { useValueLabels } from './use-value-labels';

type Sentiment = MetricDelta['sentiment'];
type InfoRow =
  | 'period' | 'buildings' | 'tariff' | 'purchasePrice' | 'averagePurchasePrice' | 'rooftopFeedIn' | 'utilityFeedIn'
  | 'consumption' | 'dailyConsumption' | 'rooftop' | 'utility' | 'production' | 'dailyProduction' | 'bill' | 'reactivePenalty';

/** A card's delta; `upIsGood` says which way is welcome (production up is, the bill up is not). */
export function metricDelta(pct: string | undefined, upIsGood: boolean, comparisonLabel: string): MetricDelta | undefined {
  const direction = deltaDirection(pct);
  if (!direction || pct === undefined) return undefined;
  const sentiment: Sentiment = direction === 'flat' ? 'neutral' : (direction === 'up') === upIsGood ? 'good' : 'bad';
  return { value: pct, direction, sentiment, comparisonLabel };
}

export function moneyIn(list: ReportMoney[], currency: string | undefined): string | null {
  return list.find((m) => m.currency === (currency ?? 'TRY'))?.value ?? null;
}

export function deltaIn(list: ReportCurrencyDelta[], currency: string | undefined): ReportDelta['pct'] {
  return list.find((d) => d.currency === (currency ?? 'TRY'))?.pct;
}

export function chartData(points: ReportMonthPoint[], monthName: (m: number) => string) {
  return points.map((p) => ({ x: monthName(p.month), current: p.current ?? null, previous: p.previous ?? null }));
}

/** The monthly report of 01 §7.14: information table, four cards, two charts. */
export function MonthlyReportView({ payload: m }: { payload: ReportMonthly }) {
  const t = useTranslations('reports');
  const locale = useLocale() as Locale;
  const l = useValueLabels();
  const lp = useValueLabels('plants');
  const monthName = (month: number) => formatMonth(`${m.year}-${String(month).padStart(2, '0')}`, locale).split(' ')[0];
  const quality: DataQuality | undefined = m.partial ? { state: 'incomplete', reason: t('partial') } : undefined;
  const currency = m.chart_currency ?? 'TRY';
  const perKwh = (v: string | undefined) => (v ? `${formatNumber(v, { minFractionDigits: 4, maxFractionDigits: 4 })} ${l.perKwh}` : l.noData);

  const info: [InfoRow, string][] = [
    ['period', formatMonth(`${m.year}-${String(m.month).padStart(2, '0')}`, locale)],
    ['buildings', m.buildings.map((b) => b.name).join(', ')],
    ['tariff', m.buildings.map((b) => `${b.name}: ${b.tariff_name ?? l.noData}`).join('; ')],
    ['purchasePrice', m.buildings.map((b) => `${b.name}: ${perKwh(b.purchase_price)}`).join('; ')],
    ['averagePurchasePrice', m.average_purchase_price.length
      ? m.average_purchase_price.map((a) => `${formatNumber(a.value, { minFractionDigits: 4, maxFractionDigits: 4 })} ${a.currency}/kWh`).join('; ')
      : l.noData],
    ['rooftopFeedIn', formatRange(m.rooftop_feed_in, l)],
    ['utilityFeedIn', formatRange(m.utility_feed_in, l)],
    ['consumption', formatFigure(m.consumption, 'kWh', l)],
    ['dailyConsumption', formatFigure(m.daily_consumption, 'kWh', l)],
    ['rooftop', formatFigure(m.rooftop, 'kWh', l)],
    ['utility', formatFigure(m.utility, 'kWh', lp)],
    ['production', formatFigure(m.production, 'kWh', l)],
    ['dailyProduction', formatFigure(m.daily_production, 'kWh', l)],
    ['bill', formatMoneyList(m.bill, l).join('; ')],
    ['reactivePenalty', formatMoneyList(m.reactive_penalty, l).join('; ')],
  ];
  const comparison = t('monthly.cards.comparison');
  const cur = String(m.year);
  const prev = String(m.year - 1);
  const omitted = m.omitted_currencies.length ? t('omittedCurrencies', { currencies: m.omitted_currencies.join(', ') }) : undefined;

  return (
    <div className="flex flex-col gap-6">
      {m.partial ? <Alert tone="info" title={t('partial')} /> : null}

      <StaggerGrid className="sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard label={t('monthly.cards.consumption')} value={m.consumption.value ?? null} unit="kWh" quality={quality}
          maxFractionDigits={2} delta={metricDelta(m.consumption_delta.pct, false, comparison)} />
        <MetricCard label={t('monthly.cards.production')} value={m.production.value ?? null} unit="kWh" quality={quality}
          maxFractionDigits={2} delta={metricDelta(m.production_delta.pct, true, comparison)} />
        <MetricCard label={t('monthly.cards.bill')} value={currency === 'TRY' ? moneyIn(m.bill, currency) : null} unit="TRY"
          delta={metricDelta(deltaIn(m.bill_delta, currency), false, comparison)} />
        <MetricCard label={t('monthly.cards.reactivePenalty')} value={currency === 'TRY' ? moneyIn(m.reactive_penalty, currency) : null} unit="TRY"
          delta={metricDelta(deltaIn(m.reactive_penalty_delta, currency), false, comparison)} />
      </StaggerGrid>

      <Card className="flex flex-col gap-3">
        <h2 className="type-h3">{t('monthly.infoTitle')}</h2>
        <TableContainer label={t('monthly.infoTitle')}>
          <Table aria-label={t('monthly.infoTitle')}>
            <TableBody>
              {info.map(([key, value]) => (
                <TableRow key={key}>
                  <TableHead scope="row" className="w-1/3">{t(`monthly.rows.${key}`)}</TableHead>
                  <TableCell>{value}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
        <p className="text-foreground-muted type-small">{t('monthly.billPeriodNote')}</p>
      </Card>

      <div className="grid gap-6 xl:grid-cols-2">
        <BarChart
          title={t('monthly.consumptionChart')}
          description={t('monthly.consumptionChartDescription')}
          xLabel={t('monthly.month')}
          data={chartData(m.consumption_chart, monthName)}
          series={[
            { key: 'current', label: cur, kind: 'current', unit: 'kWh' },
            { key: 'previous', label: prev, kind: 'previous', unit: 'kWh' },
          ]}
          empty={{ title: t('monthly.chartEmpty'), description: t('monthly.chartEmptyDescription') }}
        />
        <BarChart
          title={t('monthly.billChart')}
          description={t('monthly.billChartDescription')}
          xLabel={t('monthly.month')}
          data={chartData(m.bill_chart, monthName)}
          series={[
            { key: 'current', label: cur, kind: 'current', unit: 'TRY' },
            { key: 'previous', label: prev, kind: 'previous', unit: 'TRY' },
          ]}
          footnote={omitted}
          empty={{ title: t('monthly.chartEmpty'), description: t('monthly.chartEmptyDescription') }}
        />
      </div>
    </div>
  );
}
