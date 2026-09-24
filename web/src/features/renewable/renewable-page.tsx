'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { PeriodFilterBar, type Granularity } from '@/components/domain/period-filter-bar';
import { FilterBar } from '@/components/shell/filter-bar';
import { PageHeader } from '@/components/shell/page-header';
import { Alert } from '@/components/ui/alert';
import type { DateRange } from '@/components/ui/date-range-picker';
import { EmptyState } from '@/components/ui/empty-state';
import { Tabs } from '@/components/ui/tabs';
import { ScopePicker } from '@/features/scope/scope-picker';
import { $api } from '@/lib/api/query';
import { addDays, istanbulToday } from '@/lib/dates';
import { useScopeParams, useSelection } from '@/lib/selection/selection-store';

import { WeatherPanelView } from '../weather/weather-panel';
import { EnergyBalanceCard } from './energy-balance-card';
import {
  AnalyticsPanelView,
  EfficiencyPanelView,
  EnvironmentalPanelView,
  ForecastPanelView,
  GridPanelView,
  RealtimePanelView,
  SystemStatusPanelView,
} from './panels';
import { ProductionTabView, type ProductionToggles } from './production-tab';
import { SummaryCardsView } from './summary-cards';

/** R298: at most 366 inclusive days. */
const tooLong = (r: DateRange) => (Date.parse(r.to) - Date.parse(r.from)) / 86_400_000 > 365;

/** The renewable energy screen of 01 §7.8: every figure from meters, bills or seeded factors (R292). */
export function RenewablePage() {
  const t = useTranslations('renewable');
  const scope = useScopeParams();
  const { buildingId, analyzerId } = useSelection();
  const today = istanbulToday();

  const [activeOnly, setActiveOnly] = useState(false);
  const [tab, setTab] = useState('production');
  const [show, setShow] = useState<ProductionToggles>({
    active: true,
    inductive: false,
    capacitive: false,
  });
  const [draft, setDraft] = useState<{ granularity: Granularity; range: DateRange }>({
    granularity: 'daily',
    range: { from: addDays(today, -29), to: today },
  });
  const [applied, setApplied] = useState(draft);

  const subject = analyzerId
    ? { analyzer_id: analyzerId }
    : buildingId
      ? { building_id: buildingId }
      : null;
  const valid = !tooLong(applied.range);
  const enabled = subject !== null && valid;
  const query = { ...scope, ...subject, from: applied.range.from, to: applied.range.to };
  const detailed = enabled && tab === 'detailed';

  const overview = $api.useQuery(
    'get',
    '/api/v1/renewable/overview',
    { params: { query } },
    { enabled },
  );
  const generation = $api.useQuery(
    'get',
    '/api/v1/generation',
    { params: { query: { ...query, granularity: applied.granularity } } },
    { enabled: enabled && tab === 'production' },
  );
  const panel = { params: { query } };
  const realtime = $api.useQuery('get', '/api/v1/renewable/realtime', panel, { enabled: detailed });
  const grid = $api.useQuery('get', '/api/v1/renewable/grid-interaction', panel, {
    enabled: detailed,
  });
  const environmental = $api.useQuery('get', '/api/v1/renewable/environmental', panel, {
    enabled: detailed,
  });
  const efficiency = $api.useQuery('get', '/api/v1/renewable/efficiency', panel, {
    enabled: detailed,
  });
  const forecast = $api.useQuery('get', '/api/v1/renewable/forecast', panel, { enabled: detailed });
  const analytics = $api.useQuery('get', '/api/v1/renewable/analytics', panel, {
    enabled: detailed,
  });
  const status = $api.useQuery('get', '/api/v1/renewable/system-status', panel, {
    enabled: detailed,
  });
  const balance = $api.useQuery(
    'get',
    '/api/v1/energy-balance',
    { params: { query: { ...query, granularity: applied.granularity } } },
    { enabled: detailed },
  );
  const weather = $api.useQuery(
    'get',
    '/api/v1/weather',
    { params: { query: { ...scope, building_id: buildingId ?? '' } } },
    { enabled: detailed && Boolean(buildingId) },
  );

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />
      <FilterBar>
        <ScopePicker activeOnly={activeOnly} onActiveOnlyChange={setActiveOnly} />
        <PeriodFilterBar
          granularity={draft.granularity}
          onGranularityChange={(granularity) => setDraft((d) => ({ ...d, granularity }))}
          range={draft.range}
          onRangeChange={(range) => setDraft((d) => ({ ...d, range }))}
          onApply={() => setApplied(draft)}
          today={today}
          applying={overview.isFetching}
          allowedGranularities={['hourly', 'daily', 'monthly']}
        />
      </FilterBar>

      {subject === null ? (
        <EmptyState title={t('selectSubject')} description={t('selectSubjectHint')} />
      ) : !valid ? (
        <Alert tone="warning" title={t('rangeTooLong')} />
      ) : (
        <>
          <SummaryCardsView data={overview.data} />
          <Tabs
            value={tab}
            onValueChange={setTab}
            items={[
              {
                value: 'production',
                label: t('tabs.production'),
                content: (
                  <ProductionTabView
                    rows={generation.data?.items ?? []}
                    granularity={applied.granularity}
                    show={show}
                    onShowChange={setShow}
                    loading={generation.isFetching}
                  />
                ),
              },
              {
                value: 'detailed',
                label: t('tabs.detailed'),
                content: (
                  <div className="grid gap-4 lg:grid-cols-2">
                    <RealtimePanelView data={realtime.data} loading={realtime.isLoading} />
                    <GridPanelView data={grid.data} loading={grid.isLoading} />
                    <EnvironmentalPanelView
                      data={environmental.data}
                      loading={environmental.isLoading}
                    />
                    <EfficiencyPanelView data={efficiency.data} loading={efficiency.isLoading} />
                    <ForecastPanelView data={forecast.data} loading={forecast.isLoading} />
                    <SystemStatusPanelView data={status.data} loading={status.isLoading} />
                    <div className="lg:col-span-2">
                      <AnalyticsPanelView data={analytics.data} loading={analytics.isLoading} />
                    </div>
                    <div className="lg:col-span-2">
                      <EnergyBalanceCard
                        data={balance.data}
                        granularity={applied.granularity}
                        loading={balance.isLoading}
                      />
                    </div>
                    {buildingId ? (
                      <div className="lg:col-span-2">
                        <WeatherPanelView weather={weather.data} loading={weather.isLoading} />
                      </div>
                    ) : null}
                  </div>
                ),
              },
            ]}
          />
        </>
      )}
    </div>
  );
}
