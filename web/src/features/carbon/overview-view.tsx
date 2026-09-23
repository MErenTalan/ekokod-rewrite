'use client';

import { useLocale, useTranslations } from 'next-intl';

import { BarChart } from '@/components/charts/bar-chart';
import { FilterBar } from '@/components/shell/filter-bar';
import { Button } from '@/components/ui/button';
import { Select } from '@/components/ui/select';
import { StatTile } from '@/components/ui/stat-tile';
import { StatusBadge } from '@/components/ui/status-badge';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Locale } from '@/i18n/locale';
import type { CarbonActivity, CarbonOverview } from '@/lib/api/types';
import { formatDate, formatNumber } from '@/lib/format';

import { mainKey, scopeKey, STATUS_TONE, subKey, toTonnes } from './labels';

const kg = (v: string) => formatNumber(v, { minFractionDigits: 2, maxFractionDigits: 2 });

export type OverviewViewProps = {
  overview: CarbonOverview;
  years: number[];
  onYearChange: (year: number) => void;
  onSeeAll: () => void;
  loading?: boolean;
};

/** R322: the year's tiles, three charts with data tables, and the newest records. */
export function OverviewView({ overview, years, onYearChange, onSeeAll, loading = false }: OverviewViewProps) {
  const t = useTranslations('carbon');
  const locale = useLocale() as Locale;
  const empty = { title: t('overview.empty'), description: t('overview.emptyHint') };
  const hasData = Number(overview.total_kgco2e) !== 0;
  const period = (a: CarbonActivity) =>
    a.period_start === a.period_end
      ? formatDate(a.period_start, locale)
      : `${formatDate(a.period_start, locale)} – ${formatDate(a.period_end, locale)}`;

  return (
    <div className="flex flex-col gap-6">
      <FilterBar>
        <Select
          label={t('overview.year')}
          value={String(overview.year)}
          onValueChange={(v) => onYearChange(Number(v))}
          options={years.map((y) => ({ value: String(y), label: String(y) }))}
        />
      </FilterBar>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatTile
          label={t('overview.total')}
          value={formatNumber(toTonnes(overview.total_kgco2e), { minFractionDigits: 2, maxFractionDigits: 2 })}
          unit="tCO2e"
          hint={t('overview.totalHint', { kg: kg(overview.total_kgco2e) })}
        />
        <StatTile label={t('overview.activityCount')} value={String(overview.activity_count)} />
        <StatTile label={t('overview.registered')} value={String(overview.registered_count)} />
        <StatTile
          label={t('overview.highest')}
          value={overview.highest_source ? t(subKey(overview.highest_source.key)) : t('overview.noHighest')}
          hint={overview.highest_source ? `${kg(overview.highest_source.kgco2e)} kg CO₂e` : undefined}
        />
      </div>
      {overview.pending_count > 0 ? (
        <p className="text-foreground-muted type-small">{t('overview.pending', { count: overview.pending_count })}</p>
      ) : null}
      <div className="grid gap-4 xl:grid-cols-2">
        <BarChart
          title={t('overview.byCategory')}
          description={t('overview.byCategoryHint')}
          xLabel={t('overview.category')}
          layout="horizontal"
          loading={loading}
          data={hasData ? overview.by_category.map((c) => ({ x: t(mainKey(c.key)), value: Number(c.kgco2e) })) : []}
          series={[{ key: 'value', label: t('overview.emission'), kind: 'current', unit: 'kgCO2e' }]}
          empty={empty}
        />
        <BarChart
          title={t('overview.byScope')}
          description={t('overview.byScopeHint')}
          xLabel={t('overview.scope')}
          loading={loading}
          data={hasData ? overview.by_scope.map((s) => ({ x: t(scopeKey(s.key)), value: Number(s.kgco2e) })) : []}
          series={[{ key: 'value', label: t('overview.emission'), kind: 'current', unit: 'kgCO2e' }]}
          empty={empty}
        />
      </div>
      <BarChart
        title={t('overview.monthly')}
        description={t('overview.monthlyHint')}
        xLabel={t('overview.month')}
        loading={loading}
        data={
          overview.monthly.some((m) => Number(m.current_kgco2e) !== 0 || Number(m.previous_kgco2e) !== 0)
            ? overview.monthly.map((m) => ({
                x: t(`months.m${m.month}` as 'months.m1'),
                current: Number(m.current_kgco2e),
                previous: Number(m.previous_kgco2e),
              }))
            : []
        }
        series={[
          { key: 'current', label: t('overview.current', { year: overview.year }), kind: 'current', unit: 'kgCO2e' },
          { key: 'previous', label: t('overview.previous', { year: overview.year - 1 }), kind: 'previous', unit: 'kgCO2e' },
        ]}
        empty={empty}
      />
      <section className="flex flex-col gap-3" aria-labelledby="carbon-recent">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 id="carbon-recent" className="type-h3">
            {t('overview.recent')}
          </h2>
          <Button variant="secondary" size="sm" onClick={onSeeAll}>
            {t('overview.seeAll')}
          </Button>
        </div>
        {overview.recent.length === 0 ? (
          <p className="text-foreground-muted type-small">{t('overview.noRecent')}</p>
        ) : (
          <TableContainer label={t('overview.recent')}>
            <Table aria-label={t('overview.recent')}>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('overview.activity')}</TableHead>
                  <TableHead>{t('overview.category')}</TableHead>
                  <TableHead>{t('overview.date')}</TableHead>
                  <TableHead numeric>{t('overview.emission')}</TableHead>
                  <TableHead>{t('overview.scope')}</TableHead>
                  <TableHead>{t('tabs.status')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {overview.recent.map((a) => (
                  <TableRow key={a.id}>
                    <TableCell>
                      {t(subKey(a.sub_category))}
                      {a.is_automated ? <span className="ms-2 text-foreground-muted type-caption">{t('overview.automated')}</span> : null}
                    </TableCell>
                    <TableCell>{t(mainKey(a.main_category))}</TableCell>
                    <TableCell>{period(a)}</TableCell>
                    <TableCell numeric>{kg(a.emission_kgco2e)}</TableCell>
                    <TableCell>{t(scopeKey(a.scope))}</TableCell>
                    <TableCell>
                      <StatusBadge status={STATUS_TONE[a.status]} label={t(`status.${a.status}`)} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </section>
    </div>
  );
}
