'use client';

import { useTranslations } from 'next-intl';
import { useId } from 'react';

import { formatNumber, formatQuantity, unitSymbol, type Unit } from '@/lib/format';

import { Table, TableBody, TableCaption, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '../ui/table';
import { ChartFrame, type BaseChartProps } from './_chart-frame';
import { seriesStyle, type SeriesKind } from './_theme';

export type HeatmapChartProps = Omit<BaseChartProps, 'data' | 'series' | 'formatX'> & {
  rows: string[];
  columns: string[];
  values: (string | null)[][];
  unit: Unit;
  kind: SeriesKind;
};

const MIX = [15, 35, 55, 75, 95];
const CELL_W = 28;
const CELL_H = 20;
const LABEL_W = 44;
const LABEL_H = 18;

/** SVG grid in five colour-mix buckets of the series colour; missing cells are hatched, not blank (plan D10). */
export function HeatmapChart({ rows, columns, values, unit, kind, ...props }: HeatmapChartProps) {
  const t = useTranslations('charts');
  const hatchId = useId().replaceAll(':', '');
  const color = seriesStyle({ key: 'h', label: '', kind, unit }, 0).color;
  const numbers = values.flat().filter((v): v is string => v !== null).map(Number);
  const min = numbers.length ? Math.min(...numbers) : 0;
  const max = numbers.length ? Math.max(...numbers) : 0;
  const width = (max - min) / MIX.length || 1;
  const bucket = (v: number) => Math.min(MIX.length - 1, Math.floor((v - min) / width));
  const fill = (i: number) => `color-mix(in oklab, ${color} ${MIX[i]}%, var(--color-surface))`;
  const svgW = LABEL_W + columns.length * CELL_W;
  const svgH = LABEL_H + rows.length * CELL_H;

  return (
    <ChartFrame
      {...props}
      data={[]}
      series={[]}
      isEmpty={numbers.length === 0}
      autoHeight
      legend={
        <ul aria-label={t('legend')} className="flex flex-wrap items-center gap-x-3 gap-y-1 text-foreground-muted type-caption">
          {MIX.map((_, i) => (
            <li key={i} className="flex items-center gap-1">
              <span aria-hidden className="inline-block size-3 rounded-sm border border-border" style={{ background: fill(i) }} />
              {t('bucket', { from: formatNumber(min + i * width, { maxFractionDigits: 1 }), to: formatNumber(min + (i + 1) * width, { maxFractionDigits: 1 }) })}
            </li>
          ))}
          <li className="flex items-center gap-1">
            <svg aria-hidden width="12" height="12">
              <rect width="12" height="12" fill={`url(#${hatchId})`} stroke="var(--color-border)" />
            </svg>
            {t('noData')}
          </li>
          <li>{unitSymbol(unit)}</li>
        </ul>
      }
      table={
        <TableContainer label={props.title} className="max-h-96">
          <Table>
            <TableCaption>{props.title}</TableCaption>
            <TableHeader className="sticky top-0">
              <TableRow>
                <TableHead>{props.xLabel}</TableHead>
                {columns.map((c) => (
                  <TableHead key={c} numeric>
                    {c}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((r, i) => (
                <TableRow key={r}>
                  <TableHead scope="row">{r}</TableHead>
                  {columns.map((c, j) => (
                    <TableCell key={c} numeric>
                      {values[i]?.[j] == null ? t('noData') : formatNumber(values[i][j])}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      }
    >
      {() => (
        <svg viewBox={`0 0 ${svgW} ${svgH}`} className="w-full" aria-hidden>
          <defs>
            <pattern id={hatchId} patternUnits="userSpaceOnUse" width="6" height="6" patternTransform="rotate(45)">
              <rect width="6" height="6" fill="var(--color-surface)" />
              <line x1="0" y1="0" x2="0" y2="6" stroke="var(--color-border-strong)" strokeWidth="2" />
            </pattern>
          </defs>
          {columns.map((c, j) => (
            <text key={c} x={LABEL_W + j * CELL_W + CELL_W / 2} y={LABEL_H - 5} textAnchor="middle" fontSize="10" fill="var(--color-foreground-muted)">
              {c}
            </text>
          ))}
          {rows.map((r, i) => (
            <g key={r}>
              <text x={LABEL_W - 6} y={LABEL_H + i * CELL_H + CELL_H / 2 + 3} textAnchor="end" fontSize="10" fill="var(--color-foreground-muted)">
                {r}
              </text>
              {columns.map((c, j) => {
                const v = values[i]?.[j] ?? null;
                return (
                  <rect
                    key={c}
                    data-cell
                    x={LABEL_W + j * CELL_W + 1}
                    y={LABEL_H + i * CELL_H + 1}
                    width={CELL_W - 2}
                    height={CELL_H - 2}
                    rx="2"
                    fill={v === null ? `url(#${hatchId})` : undefined}
                    style={v === null ? undefined : { fill: fill(bucket(Number(v))) }}
                  >
                    <title>{`${r} ${c}: ${v === null ? t('noData') : formatQuantity(v, unit)}`}</title>
                  </rect>
                );
              })}
            </g>
          ))}
        </svg>
      )}
    </ChartFrame>
  );
}
