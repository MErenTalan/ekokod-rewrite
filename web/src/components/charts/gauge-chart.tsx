'use client';

import { AlertTriangle, XCircle } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { cn } from '@/lib/cn';
import { formatNumber, formatQuantity, type Unit } from '@/lib/format';

import { Table, TableBody, TableCaption, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '../ui/table';
import { ChartFrame, type BaseChartProps } from './_chart-frame';

export type GaugeThreshold = { value: number; label: string; status: 'warning' | 'danger' };
export type GaugeChartProps = Omit<BaseChartProps, 'data' | 'series' | 'xLabel' | 'formatX'> & {
  value: string | null;
  min: number;
  max: number;
  unit: Unit;
  thresholds: GaugeThreshold[];
  label: string;
};

const CX = 100;
const CY = 100;
const R = 80;
const point = (f: number) => {
  const angle = Math.PI * (1 - Math.min(1, Math.max(0, f)));
  return { x: CX + R * Math.cos(angle), y: CY - R * Math.sin(angle) };
};
const arc = (from: number, to: number) => {
  const a = point(from);
  const b = point(to);
  return `M ${a.x} ${a.y} A ${R} ${R} 0 0 1 ${b.x} ${b.y}`;
};

/** Hand-built semicircle (plan D10): threshold ticks in SVG, threshold labels as text beneath. */
export function GaugeChart({ value, min, max, unit, thresholds, label, ...props }: GaugeChartProps) {
  const t = useTranslations('charts');
  const numeric = value === null ? null : Number(value);
  const fraction = (v: number) => (v - min) / (max - min || 1);
  const crossed = [...thresholds].sort((a, b) => b.value - a.value).find((th) => numeric !== null && numeric >= th.value);
  const valueColor = crossed ? `var(--color-${crossed.status})` : 'var(--color-success)';
  const shown = value === null ? t('noData') : formatQuantity(value, unit);

  return (
    <ChartFrame
      {...props}
      data={[]}
      series={[]}
      xLabel={label}
      isEmpty={false}
      autoHeight
      legend={<span />}
      table={
        <TableContainer label={props.title}>
          <Table>
            <TableCaption>{props.title}</TableCaption>
            <TableHeader>
              <TableRow>
                <TableHead>{label}</TableHead>
                <TableHead numeric>{t('value')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow>
                <TableCell>{label}</TableCell>
                <TableCell numeric>{shown}</TableCell>
              </TableRow>
              {thresholds.map((th) => (
                <TableRow key={th.value}>
                  <TableCell>{th.label}</TableCell>
                  <TableCell numeric>{formatQuantity(String(th.value), unit)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      }
    >
      {() => (
        <div className="flex flex-col items-center gap-2">
          <svg viewBox="0 0 200 116" role="img" aria-label={`${label}: ${shown}`} className="w-full max-w-xs">
            <path d={arc(0, 1)} fill="none" stroke="var(--color-surface-sunken)" strokeWidth={16} />
            {numeric !== null ? <path d={arc(0, fraction(numeric))} fill="none" stroke={valueColor} strokeWidth={16} /> : null}
            {thresholds.map((th) => {
              const inner = point(fraction(th.value));
              const outer = { x: CX + (inner.x - CX) * 1.18, y: CY + (inner.y - CY) * 1.18 };
              const innerEdge = { x: CX + (inner.x - CX) * 0.82, y: CY + (inner.y - CY) * 0.82 };
              return <line key={th.value} x1={innerEdge.x} y1={innerEdge.y} x2={outer.x} y2={outer.y} stroke="var(--color-foreground)" strokeWidth={2} />;
            })}
            <text x={CX} y={CY - 8} textAnchor="middle" className="fill-foreground" style={{ font: '600 20px var(--font-mono)' }}>
              {shown}
            </text>
            <text x={CX} y={CY + 12} textAnchor="middle" className="fill-foreground-muted" style={{ font: '400 10px var(--font-sans)' }}>
              {formatNumber(min)} – {formatNumber(max)}
            </text>
          </svg>
          <p className="text-foreground type-small font-semibold">{label}</p>
          <ul className="flex flex-wrap justify-center gap-x-4 gap-y-1 type-small">
            {thresholds.map((th) => {
              const hit = numeric !== null && numeric >= th.value;
              const Icon = th.status === 'danger' ? XCircle : AlertTriangle;
              return (
                <li key={th.value} className={cn('flex items-center gap-1', hit ? 'font-semibold text-foreground' : 'text-foreground-muted')}>
                  {hit ? <Icon aria-hidden className={cn('size-4', th.status === 'danger' ? 'text-danger' : 'text-warning')} /> : null}
                  {th.label}
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </ChartFrame>
  );
}
