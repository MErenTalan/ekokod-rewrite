'use client';

import { Bar, BarChart as RBarChart, CartesianGrid, Cell, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';

import { formatNumber, unitSymbol } from '@/lib/format';

import { ChartFrame, type BaseChartProps } from './_chart-frame';
import { ChartDataTable } from './_chart-data-table';
import { ChartTooltip } from './_chart-tooltip';
import { AXIS, chartAnimation, CURSOR, GRID, seriesStyle, toPlot, toPlotData, type Datum } from './_theme';
import { usePrefersReducedMotion } from './use-reduced-motion';

export type BarChartProps = BaseChartProps & {
  /** `horizontal` draws horizontal bars with categories on the vertical axis. */
  layout?: 'vertical' | 'horizontal';
  sort?: 'desc';
  referenceLine?: { value: number; label: string };
  /** One value per category around zero; positive uses the consumption series' colour, negative the generation one. */
  diverging?: boolean;
};

const REFERENCE = { stroke: 'var(--color-foreground-muted)', dash: '4 4' };

export function BarChart({ layout = 'vertical', sort, referenceLine, diverging = false, ...props }: BarChartProps) {
  const reduced = usePrefersReducedMotion();
  const { series, height = 320, formatX, title, description } = props;
  const first = series[0];
  const data: Datum[] =
    sort === 'desc' && first ? [...props.data].sort((a, b) => (toPlot(b[first.key]) ?? -Infinity) - (toPlot(a[first.key]) ?? -Infinity)) : props.data;
  const horizontal = layout === 'horizontal';
  const valueAxis = {
    ...AXIS,
    type: 'number' as const,
    tickFormatter: (v: number) => formatNumber(v, { maxFractionDigits: 1 }),
    label: { value: unitSymbol(first?.unit ?? 'kWh'), fill: 'var(--color-foreground-muted)', fontSize: 12, ...(horizontal ? { position: 'insideBottomRight' as const, dy: 8 } : { angle: -90, position: 'insideLeft' as const }) },
  };
  const categoryAxis = { ...AXIS, type: 'category' as const, dataKey: 'x', tickFormatter: formatX };
  const drawn = diverging ? series.slice(0, 1) : series;
  const positive = series.find((s) => s.kind === 'consumption') ?? first;
  const negative = series.find((s) => s.kind === 'generation') ?? first;
  const reference = referenceLine ? (horizontal ? { x: referenceLine.value } : { y: referenceLine.value }) : null;

  return (
    <ChartFrame
      {...props}
      data={data}
      legendExtra={referenceLine ? [{ label: referenceLine.label, color: REFERENCE.stroke, dash: REFERENCE.dash }] : undefined}
      table={diverging && first ? <ChartDataTable caption={title} xLabel={props.xLabel} series={[{ ...first, label: series.map((s) => s.label).join(' / ') }]} data={data} formatX={formatX} /> : undefined}
    >
      {(plot) => (
        <ResponsiveContainer width="100%" height={height} initialDimension={{ width: 640, height }}>
          <RBarChart
            data={toPlotData(plot, drawn.map((s) => s.key))}
            layout={horizontal ? 'vertical' : 'horizontal'}
            margin={{ top: 8, right: 16, bottom: horizontal ? 16 : 4, left: 4 }}
            accessibilityLayer
            title={title}
            desc={description}
          >
            <CartesianGrid {...GRID} horizontal={!horizontal} vertical={horizontal} />
            {horizontal ? <XAxis {...valueAxis} /> : <XAxis {...categoryAxis} />}
            {horizontal ? <YAxis {...categoryAxis} width={112} /> : <YAxis {...valueAxis} width={64} />}
            <Tooltip cursor={CURSOR} content={<ChartTooltip series={drawn} formatX={formatX} />} />
            {diverging ? <ReferenceLine {...(horizontal ? { x: 0 } : { y: 0 })} stroke="var(--color-foreground-muted)" /> : null}
            {reference ? <ReferenceLine {...reference} stroke={REFERENCE.stroke} strokeDasharray={REFERENCE.dash} strokeWidth={1.5} /> : null}
            {drawn.map((s, i) => (
              <Bar key={s.key} dataKey={s.key} name={s.label} fill={seriesStyle(s, i).color} radius={2} maxBarSize={48} {...chartAnimation(reduced)}>
                {diverging
                  ? plot.map((d, j) => <Cell key={j} fill={seriesStyle((toPlot(d[s.key]) ?? 0) < 0 ? negative : positive, 0).color} />)
                  : null}
              </Bar>
            ))}
          </RBarChart>
        </ResponsiveContainer>
      )}
    </ChartFrame>
  );
}
