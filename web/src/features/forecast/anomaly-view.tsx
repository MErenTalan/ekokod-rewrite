'use client';

import { useTranslations } from 'next-intl';

import { BarChart } from '@/components/charts/bar-chart';
import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableRow } from '@/components/ui/table';
import type { components } from '@/lib/api/schema';
import { formatNumber } from '@/lib/format';

import { METHOD_KEY } from './forecast-status';

export type AnomalyResult = components['schemas']['AnomalyCheck'];

const num = (v: string | number | null | undefined) => (v === null || v === undefined ? '—' : formatNumber(Number(v), { maxFractionDigits: 2 }));

/** 01 §7.6 anomaly check: the verdict with its score, band and method, as a table and a chart (Q-I18). */
export function AnomalyView({ result }: { result: AnomalyResult }) {
  const t = useTranslations('forecast.anomaly');
  const insufficient = result.method === 'insufficient_history';
  const method = result.method && METHOD_KEY[result.method] ? t(`methods.${METHOD_KEY[result.method]}`) : (result.method ?? '—');
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-2" role="status">
        {insufficient ? (
          <span className="text-foreground-muted">{t('insufficient')}</span>
        ) : (
          <Badge tone={result.is_anomaly ? 'danger' : 'success'}>{result.is_anomaly ? t('anomalous') : t('normal')}</Badge>
        )}
      </div>
      <TableContainer label={t('chartTitle')}>
        <Table aria-label={t('chartTitle')}>
          <TableBody>
            <TableRow>
              <TableHead scope="row">{t('actualUsed')}</TableHead>
              <TableCell numeric>{num(result.actual)} kWh</TableCell>
            </TableRow>
            <TableRow>
              <TableHead scope="row">{t('expected')}</TableHead>
              <TableCell numeric>{num(result.expected)} kWh</TableCell>
            </TableRow>
            <TableRow>
              <TableHead scope="row">{t('band')}</TableHead>
              <TableCell numeric>{`${num(result.lower)} – ${num(result.upper)}`}</TableCell>
            </TableRow>
            <TableRow>
              <TableHead scope="row">{t('score')}</TableHead>
              <TableCell numeric>{num(result.score)}</TableCell>
            </TableRow>
            <TableRow>
              <TableHead scope="row">{t('method')}</TableHead>
              <TableCell>{method}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </TableContainer>
      {!insufficient ? (
        <BarChart
          title={t('chartTitle')}
          description={`${t('band')}: ${num(result.lower)} – ${num(result.upper)} kWh`}
          data={[
            { x: t('actualUsed'), value: result.actual ?? null },
            { x: t('expected'), value: result.expected ?? null },
          ]}
          series={[{ key: 'value', label: 'kWh', kind: 'consumption', unit: 'kWh' }]}
          xLabel={t('method')}
          height={220}
          empty={{ title: t('insufficient'), description: '' }}
        />
      ) : null}
    </div>
  );
}
