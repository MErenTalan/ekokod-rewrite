'use client';

import { useLocale, useTranslations } from 'next-intl';

import { LineChart } from '@/components/charts/line-chart';
import { Table, TableBody, TableCell, TableContainer, TableFooter, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Locale } from '@/i18n/locale';
import { formatDateTime, formatNumber } from '@/lib/format';

import type { ForecastPoint } from './prediction';

export type PredictionRow = { label: string; median: string; p10?: string | null; p90?: string | null };
export type PredictionResultProps = {
  title: string;
  points: ForecastPoint[];
  rows: PredictionRow[];
  firstColumn: string;
  summary?: string;
  notStored?: boolean;
};

const sum = (rows: PredictionRow[], key: 'median' | 'p10' | 'p90') => rows.reduce((s, r) => s + Number(r[key] ?? r.median), 0);

/** R384: every AI result as a chart (median + p10–p90 band) and as a table with its total. */
export function PredictionResult({ title, points, rows, firstColumn, summary, notStored }: PredictionResultProps) {
  const t = useTranslations('forecast');
  const locale = useLocale() as Locale;
  const kwh = (v: number | string | null | undefined) => (v === null || v === undefined ? '—' : formatNumber(Number(v), { maxFractionDigits: 2 }));
  return (
    <div className="flex flex-col gap-4">
      {summary ? <p role="status" className="text-foreground type-body-lg font-semibold">{summary}</p> : null}
      {notStored ? <p className="text-foreground-muted type-small">{t('ai.notStored')}</p> : null}
      <LineChart
        title={title}
        description={t('predict.chartDescription')}
        data={points.map((p) => ({ x: p.ts, median: p.median, p10: p.p10 ?? null, p90: p.p90 ?? null }))}
        series={[{ key: 'median', label: t('predict.median'), kind: 'forecast', unit: 'kWh' }]}
        band={{ lowerKey: 'p10', upperKey: 'p90', label: t('predict.band') }}
        xLabel={firstColumn}
        formatX={(x) => formatDateTime(String(x), locale)}
        directLabels={false}
        empty={{ title: t('status.empty'), description: t('predict.chartDescription') }}
      />
      <TableContainer label={title}>
        <Table aria-label={title}>
          <TableHeader>
            <TableRow>
              <TableHead>{firstColumn}</TableHead>
              <TableHead numeric>{t('predict.median')}</TableHead>
              <TableHead numeric>{t('predict.band')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r) => (
              <TableRow key={r.label}>
                <TableCell>{r.label}</TableCell>
                <TableCell numeric>{kwh(r.median)}</TableCell>
                <TableCell numeric>{`${kwh(r.p10)} – ${kwh(r.p90)}`}</TableCell>
              </TableRow>
            ))}
          </TableBody>
          <TableFooter>
            <TableRow>
              <TableCell className="font-semibold">{t('ai.total')}</TableCell>
              <TableCell numeric className="font-semibold">{kwh(sum(rows, 'median'))}</TableCell>
              <TableCell numeric>{`${kwh(sum(rows, 'p10'))} – ${kwh(sum(rows, 'p90'))}`}</TableCell>
            </TableRow>
          </TableFooter>
        </Table>
      </TableContainer>
    </div>
  );
}
