'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { FilterBar } from '@/components/shell/filter-bar';
import { PageHeader } from '@/components/shell/page-header';
import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { DateRangePicker, type DateRange } from '@/components/ui/date-range-picker';
import { Select } from '@/components/ui/select';
import { ScopePicker } from '@/features/scope/scope-picker';
import { errorCodeOf } from '@/lib/api/problem';
import { $api } from '@/lib/api/query';
import { addDays, istanbulToday } from '@/lib/dates';
import { useScopeParams, useSelection } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import type { Forecast } from './forecast-status';
import { ForecastView } from './forecast-view';

const HORIZONS = [24, 48, 168, 336, 744] as const;
const HOUR = 3_600_000;

/** The Predict screen of 01 §7.5 on F13b's API: stored runs are read, new ones need forecast.run. */
export function PredictPage() {
  const t = useTranslations('forecast.predict');
  const { can } = useSession();
  const scope = useScopeParams();
  const { analyzerId } = useSelection();
  const today = istanbulToday();

  const [activeOnly, setActiveOnly] = useState(false);
  const [history, setHistory] = useState<DateRange>({ from: addDays(today, -7), to: today });
  const [horizon, setHorizon] = useState<number>(48);
  const [ran, setRan] = useState<Forecast | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  // The window starts at the current hour; it is fixed per horizon so the query key is stable.
  const [origin] = useState(() => Math.floor(Date.now() / HOUR) * HOUR);

  const enabled = Boolean(analyzerId);
  const actuals = $api.useQuery('get', '/api/v1/consumption', {
    params: { query: { ...scope, analyzer_id: analyzerId ?? '', granularity: 'hourly', from: history.from, to: history.to } },
  }, { enabled });
  const stored = $api.useQuery('get', '/api/v1/forecast', {
    params: { query: { ...scope, analyzer_id: analyzerId ?? '', from: new Date(origin).toISOString(), to: new Date(origin + horizon * HOUR).toISOString() } },
  }, { enabled });
  const run = $api.useMutation('post', '/api/v1/forecast/run');

  const onRun = () => {
    if (!analyzerId) return;
    setUnavailable(false);
    run.mutate({ params: { query: scope }, body: { analyzer_id: analyzerId, horizon_hours: horizon } }, {
      onSuccess: (res) => setRan(res ?? null),
      onError: (err) => setUnavailable(errorCodeOf(err) === 'forecast_unavailable'),
    });
  };

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />
      <FilterBar>
        <ScopePicker activeOnly={activeOnly} onActiveOnlyChange={setActiveOnly} />
        <div className="w-80 max-w-full">
          <DateRangePicker label={t('history')} value={history} onValueChange={setHistory} max={today} />
        </div>
        <div className="w-40">
          <Select
            label={t('horizon')}
            value={String(horizon)}
            options={HORIZONS.map((h) => ({ value: String(h), label: t(`horizons.h${h}`) }))}
            onValueChange={(v) => {
              setHorizon(Number(v));
              setRan(null);
            }}
          />
        </div>
        {can('forecast.run') ? (
          <Button onClick={onRun} loading={run.isPending} disabled={!enabled}>
            {t('run')}
          </Button>
        ) : null}
      </FilterBar>
      {!can('forecast.run') ? <p className="text-foreground-muted type-small">{t('readOnly')}</p> : null}
      {!enabled ? (
        <Alert tone="info" title={t('noAnalyzer')} />
      ) : (
        <ForecastView
          actuals={(actuals.data?.items ?? []).map((row) => ({ ts: row.period_start, value: row.active_import ?? null }))}
          forecast={ran ?? stored.data ?? null}
          unavailable={unavailable}
          loading={actuals.isLoading || stored.isLoading}
        />
      )}
    </div>
  );
}
