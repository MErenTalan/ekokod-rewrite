'use client';

import { useLocale, useTranslations } from 'next-intl';
import { useState, type ReactNode } from 'react';

import { FilterBar } from '@/components/shell/filter-bar';
import { PageHeader } from '@/components/shell/page-header';
import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { DatePicker } from '@/components/ui/date-picker';
import { MonthPicker } from '@/components/ui/month-picker';
import { NumberInput } from '@/components/ui/number-input';
import { Select } from '@/components/ui/select';
import { Tabs } from '@/components/ui/tabs';
import { ScopePicker } from '@/features/scope/scope-picker';
import type { Locale } from '@/i18n/locale';
import { errorCodeOf, fieldErrors } from '@/lib/api/problem';
import { $api } from '@/lib/api/query';
import type { Translator } from '@/lib/api/types';
import { addDays, formatHour, istanbulToday, monthOf, startOfWeek } from '@/lib/dates';
import { formatDate, formatNumber } from '@/lib/format';
import { useScopeParams, useSelection } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { AnomalyView, type AnomalyResult } from './anomaly-view';
import { forecastStatus, STATE_KEY, type Forecast } from './forecast-status';
import { dailyTotals, hoursUntilEndOf, pointsOn, sumBand, type ForecastPoint } from './prediction';
import { PredictionResult, type PredictionRow } from './prediction-result';

const HOUR_LABEL = new Intl.DateTimeFormat('tr-TR', { hour: '2-digit', minute: '2-digit', timeZone: 'Europe/Istanbul' });

type Outcome = { forecast: Forecast | null; unavailable: boolean; errors: Record<string, string> };
const IDLE: Outcome = { forecast: null, unavailable: false, errors: {} };

/** One forecast call's life: result, unavailability (503) and field errors (422). */
function useOutcome() {
  const forms = useTranslations('forms') as unknown as Translator;
  const [outcome, setOutcome] = useState<Outcome>(IDLE);
  return {
    outcome,
    handlers: {
      onSuccess: (res: Forecast | undefined) => setOutcome({ ...IDLE, forecast: res ?? null }),
      onError: (err: unknown) => setOutcome({ forecast: null, unavailable: errorCodeOf(err) === 'forecast_unavailable', errors: fieldErrors(err, forms) }),
    },
    reset: () => setOutcome(IDLE),
  };
}

function Result({ outcome, children }: { outcome: Outcome; children: (points: ForecastPoint[]) => ReactNode }) {
  const t = useTranslations('forecast');
  if (!outcome.forecast && !outcome.unavailable) return null;
  const state = forecastStatus(outcome.forecast, outcome.unavailable);
  if (state !== 'ok') return <Alert tone={state === 'model_error' ? 'danger' : 'warning'} title={t(`status.${STATE_KEY[state]}`)} />;
  return <>{children(outcome.forecast!.points)}</>;
}

/** 01 §7.6 AI Analysis: daily, weekly and monthly predictions and the anomaly check, each as table and chart. */
export function AiPage() {
  const t = useTranslations('forecast');
  const locale = useLocale() as Locale;
  const { can } = useSession();
  const { analyzerId } = useSelection();
  const scope = useScopeParams();
  const today = istanbulToday();
  const [activeOnly, setActiveOnly] = useState(false);
  const [tab, setTab] = useState('daily');

  const run = $api.useMutation('post', '/api/v1/forecast/run');
  const weekly = $api.useMutation('post', '/api/v1/forecast/weekly');
  const monthly = $api.useMutation('post', '/api/v1/forecast/monthly');
  const anomaly = $api.useMutation('post', '/api/v1/anomaly/check');

  const [day, setDay] = useState<string | null>(addDays(today, 1));
  const nextMonday = addDays(startOfWeek(today), 7);
  const [week, setWeek] = useState<string | null>(nextMonday);
  const [month, setMonth] = useState<string | null>(monthOf(today));
  const [checkDay, setCheckDay] = useState<string | null>(today);
  const previousHour = (Number(HOUR_LABEL.format(new Date()).slice(0, 2)) + 23) % 24;
  const [checkHour, setCheckHour] = useState(String(previousHour));
  const [actual, setActual] = useState<string | null>(null);
  const [verdict, setVerdict] = useState<AnomalyResult | null>(null);
  const [checkErrors, setCheckErrors] = useState<Record<string, string>>({});
  const forms = useTranslations('forms') as unknown as Translator;

  const daily = useOutcome();
  const week7 = useOutcome();
  const month31 = useOutcome();
  const kwh = (v: string) => formatNumber(Number(v), { maxFractionDigits: 2 });
  const dayRows = (points: ForecastPoint[]): PredictionRow[] => points.map((p) => ({ label: HOUR_LABEL.format(new Date(p.ts)), median: p.median, p10: p.p10, p90: p.p90 }));
  const dateRows = (points: ForecastPoint[]): PredictionRow[] => dailyTotals(points).map((d) => ({ label: formatDate(d.date, locale), median: d.median, p10: d.p10, p90: d.p90 }));

  if (!can('forecast.run')) {
    return (
      <div className="flex flex-col gap-6">
        <PageHeader title={t('ai.title')} description={t('ai.subtitle')} />
        <Alert tone="info" title={t('predict.readOnly')} />
      </div>
    );
  }

  const body = { analyzer_id: analyzerId ?? '' };
  const disabled = !analyzerId;
  const runButton = (onClick: () => void, pending: boolean) => (
    <Button onClick={onClick} loading={pending} disabled={disabled} className="self-end">{t('ai.run')}</Button>
  );

  const items = [
    {
      value: 'daily',
      label: t('ai.tabs.daily'),
      content: (
        <div className="flex flex-col gap-4 pt-4">
          <div className="flex flex-wrap items-end gap-3">
            <div className="w-56">
              <DatePicker label={t('ai.targetDate')} value={day} onValueChange={setDay} min={today} max={addDays(today, 30)} error={daily.outcome.errors.horizon_hours} />
            </div>
            {runButton(() => day && run.mutate({ params: { query: scope }, body: { ...body, horizon_hours: hoursUntilEndOf(day, Date.now()) } }, daily.handlers), run.isPending)}
          </div>
          <Result outcome={daily.outcome}>
            {(points) => {
              const own = pointsOn(points, day ?? '');
              const total = sumBand(own);
              const weekday = new Intl.DateTimeFormat(locale === 'tr' ? 'tr-TR' : 'en-US', { weekday: 'long', timeZone: 'Europe/Istanbul' }).format(new Date(`${day}T12:00:00+03:00`));
              return (
                <PredictionResult title={t('ai.tabs.daily')} points={own} rows={dayRows(own)} firstColumn={t('ai.hour')}
                  summary={`${t('ai.dayTotal', { weekday })}: ${kwh(total.median)} kWh · ${t('ai.approxBand', { low: kwh(total.p10), high: kwh(total.p90) })}`} />
              );
            }}
          </Result>
        </div>
      ),
    },
    {
      value: 'weekly',
      label: t('ai.tabs.weekly'),
      content: (
        <div className="flex flex-col gap-4 pt-4">
          <div className="flex flex-wrap items-end gap-3">
            <div className="w-56">
              <DatePicker label={t('ai.weekStart')} value={week} onValueChange={setWeek} min={startOfWeek(today)} max={addDays(today, 30)} error={week7.outcome.errors.week_start} />
            </div>
            {runButton(() => week && weekly.mutate({ params: { query: scope }, body: { ...body, week_start: week } }, week7.handlers), weekly.isPending)}
          </div>
          <Result outcome={week7.outcome}>
            {(points) => <PredictionResult title={t('ai.tabs.weekly')} points={points} rows={dateRows(points)} firstColumn={t('ai.day')} notStored />}
          </Result>
        </div>
      ),
    },
    {
      value: 'monthly',
      label: t('ai.tabs.monthly'),
      content: (
        <div className="flex flex-col gap-4 pt-4">
          <div className="flex flex-wrap items-end gap-3">
            <div className="w-56">
              <MonthPicker label={t('ai.month')} value={month} onValueChange={setMonth} min={monthOf(today)} error={month31.outcome.errors.month} />
            </div>
            {runButton(() => month && monthly.mutate({ params: { query: scope }, body: { ...body, month } }, month31.handlers), monthly.isPending)}
          </div>
          <Result outcome={month31.outcome}>
            {(points) => <PredictionResult title={t('ai.tabs.monthly')} points={points} rows={dateRows(points)} firstColumn={t('ai.day')} notStored />}
          </Result>
        </div>
      ),
    },
    {
      value: 'anomaly',
      label: t('ai.tabs.anomaly'),
      content: (
        <div className="flex flex-col gap-4 pt-4">
          <div className="flex flex-wrap items-end gap-3">
            <div className="w-56">
              <DatePicker label={t('anomaly.ts')} value={checkDay} onValueChange={setCheckDay} max={today} error={checkErrors.ts} />
            </div>
            <div className="w-32">
              <Select label={t('anomaly.hour')} value={checkHour} onValueChange={setCheckHour}
                options={Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: formatHour(h) }))} />
            </div>
            <div className="w-56">
              <NumberInput label={t('anomaly.actual')} description={t('anomaly.actualHint')} value={actual} onValueChange={setActual} min="0" fractionDigits={3} error={checkErrors.actual} />
            </div>
            <Button className="self-end" disabled={disabled || !checkDay} loading={anomaly.isPending}
              onClick={() => {
                const ts = `${checkDay}T${String(checkHour).padStart(2, '0')}:00:00+03:00`;
                anomaly.mutate({ params: { query: scope }, body: { ...body, ts, ...(actual ? { actual } : {}) } }, {
                  onSuccess: (res) => { setVerdict(res ?? null); setCheckErrors({}); },
                  onError: (err) => { setVerdict(null); setCheckErrors(fieldErrors(err, forms)); },
                });
              }}>
              {t('anomaly.check')}
            </Button>
          </div>
          {verdict && !verdict.available ? <Alert tone="warning" title={t('status.unavailable')} /> : null}
          {verdict?.available ? <AnomalyView result={verdict} /> : null}
        </div>
      ),
    },
  ];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('ai.title')} description={t('ai.subtitle')} />
      <FilterBar>
        <ScopePicker activeOnly={activeOnly} onActiveOnlyChange={setActiveOnly} />
      </FilterBar>
      {disabled ? <Alert tone="info" title={t('predict.noAnalyzer')} /> : null}
      <Tabs items={items} value={tab} onValueChange={setTab} />
    </div>
  );
}
