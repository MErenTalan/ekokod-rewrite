'use client';

import { useTranslations } from 'next-intl';

import { formatNumber, unitSymbol } from '@/lib/format';

import { Table, TableBody, TableCaption, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '../ui/table';
import type { ChartSeries, Datum } from './_theme';

export type ChartDataTableProps = {
  caption: string;
  xLabel: string;
  series: ChartSeries[];
  data: Datum[];
  formatX?: (x: string | number) => string;
};

/** The chart's accessible equivalent, always over the full (not downsampled) data (07 §5, §9). */
export function ChartDataTable({ caption, xLabel, series, data, formatX }: ChartDataTableProps) {
  const t = useTranslations('charts');
  return (
    <TableContainer label={caption} className="max-h-96">
      <Table>
        <TableCaption>{caption}</TableCaption>
        <TableHeader className="sticky top-0">
          <TableRow>
            <TableHead>{xLabel}</TableHead>
            {series.map((s) => (
              <TableHead key={s.key} numeric>
                {t('seriesWithUnit', { label: s.label, unit: unitSymbol(s.unit) })}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {data.map((d, i) => (
            <TableRow key={`${d.x}-${i}`}>
              <TableCell>{formatX ? formatX(d.x) : d.x}</TableCell>
              {series.map((s) => (
                <TableCell key={s.key} numeric>
                  {formatNumber(d[s.key])}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}
