'use client';

import { useLocale, useTranslations } from 'next-intl';

import { LineChart } from '@/components/charts/line-chart';
import { Alert, type AlertTone } from '@/components/ui/alert';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Locale } from '@/i18n/locale';
import { formatDateTime } from '@/lib/format';

import { chartRows, COVARIATE_KEY, forecastStatus, STATE_KEY, totalGapHours, type Actual, type Forecast, type ForecastState } from './forecast-status';

export type ForecastViewProps = { actuals: Actual[]; forecast: Forecast | null; unavailable: boolean; loading: boolean };

const TONE: Record<ForecastState, AlertTone | null> = {
  ok: null, empty: 'warning', none: 'info', insufficient_data: 'warning', no_data: 'warning', model_error: 'danger', unavailable: 'warning',
};

/** 01 §7.5: actuals with the forecast median and its p10–p90 band, the model used and the gaps it saw. */
export function ForecastView({ actuals, forecast, unavailable, loading }: ForecastViewProps) {
  const t = useTranslations('forecast');
  const locale = useLocale() as Locale;
  const state = forecastStatus(forecast, unavailable);
  const tone = TONE[state];
  const gaps = unavailable ? [] : (forecast?.gaps ?? []);
  const shown = unavailable ? null : forecast;
  const hourLabel = (x: string | number) => formatDateTime(String(x), locale);
  const covariates = shown?.used_covariates.map((c) => (COVARIATE_KEY[c] ? t(`model.names.${COVARIATE_KEY[c]}`) : c)) ?? [];

  return (
    <div className="flex flex-col gap-4">
      {tone ? <Alert tone={tone} title={t(`status.${STATE_KEY[state]}`)} /> : null}
      <LineChart
        title={t('predict.chartTitle')}
        description={t('predict.chartDescription')}
        data={chartRows(actuals, shown)}
        series={[
          { key: 'actual', label: t('predict.actual'), kind: 'consumption', unit: 'kWh' },
          { key: 'median', label: t('predict.median'), kind: 'forecast', unit: 'kWh' },
        ]}
        band={{ lowerKey: 'p10', upperKey: 'p90', label: t('predict.band') }}
        xLabel={t('anomaly.hour')}
        formatX={hourLabel}
        loading={loading}
        directLabels={false}
        empty={{ title: t('status.none'), description: t('predict.chartDescription') }}
      />
      {shown?.model_id ? (
        <div className="flex flex-col gap-1 text-foreground-muted type-small">
          <p>
            {t('model.line', { model: shown.model_id, version: shown.model_version ?? '', generated: shown.generated_at ? formatDateTime(shown.generated_at, locale) : '—' })}
          </p>
          {shown.fallback_from ? <p>{t('model.fallback', { model: shown.fallback_from })}</p> : null}
          <p>{t('model.covariates', { list: covariates.length ? covariates.join(', ') : t('model.none') })}</p>
        </div>
      ) : null}
      {gaps.length > 0 ? (
        <div className="flex flex-col gap-2">
          <h2 id="forecast-gaps" className="text-foreground type-h3">{t('gaps.title')}</h2>
          <Alert tone="warning" title={t('gaps.warning', { count: gaps.length, hours: totalGapHours(gaps) })} />
          <TableContainer label={t('gaps.title')}>
            <Table aria-labelledby="forecast-gaps">
              <TableHeader>
                <TableRow>
                  <TableHead>{t('gaps.start')}</TableHead>
                  <TableHead>{t('gaps.end')}</TableHead>
                  <TableHead numeric>{t('gaps.hours')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {gaps.map((g) => (
                  <TableRow key={g.start}>
                    <TableCell>{formatDateTime(g.start, locale)}</TableCell>
                    <TableCell>{formatDateTime(g.end, locale)}</TableCell>
                    <TableCell numeric>{g.missing_hours}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </div>
      ) : null}
    </div>
  );
}
