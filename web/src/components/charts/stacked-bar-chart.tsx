'use client';

import { Bar, BarChart as RBarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';

import { formatNumber, unitSymbol } from '@/lib/format';

import { ChartFrame, type BaseChartProps } from './_chart-frame';
import { ChartTooltip } from './_chart-tooltip';
import { AXIS, chartAnimation, CURSOR, GRID, seriesStyle, toPlotData } from './_theme';
import { usePrefersReducedMotion } from './use-reduced-motion';

export type StackedBarChartProps = BaseChartProps & { layout?: 'vertical' | 'horizontal' };

/** One stack per category; a 1 px surface stroke separates segments so adjacent colours never merge. */
export function StackedBarChart({ layout = 'vertical', ...props }: StackedBarChartProps) {
  const reduced = usePrefersReducedMotion();
  const { series, height = 320, formatX, title, description } = props;
  const horizontal = layout === 'horizontal';
  const valueAxis = {
    ...AXIS,
    type: 'number' as const,
    tickFormatter: (v: number) => formatNumber(v, { maxFractionDigits: 1 }),
    label: { value: unitSymbol(series[0]?.unit ?? 'kWh'), fill: 'var(--color-foreground-muted)', fontSize: 12, ...(horizontal ? { position: 'insideBottomRight' as const, dy: 8 } : { angle: -90, position: 'insideLeft' as const }) },
  };
  const categoryAxis = { ...AXIS, type: 'category' as const, dataKey: 'x', tickFormatter: formatX };
  return (
    <ChartFrame {...props}>
      {(plot) => (
        <ResponsiveContainer width="100%" height={height} initialDimension={{ width: 640, height }}>
          <RBarChart
            data={toPlotData(plot, series.map((s) => s.key))}
            layout={horizontal ? 'vertical' : 'horizontal'}
            margin={{ top: 8, right: 16, bottom: horizontal ? 16 : 4, left: 4 }}
            accessibilityLayer
            title={title}
            desc={description}
          >
            <CartesianGrid {...GRID} horizontal={!horizontal} vertical={horizontal} />
            {horizontal ? <XAxis {...valueAxis} /> : <XAxis {...categoryAxis} />}
            {horizontal ? <YAxis {...categoryAxis} width={112} /> : <YAxis {...valueAxis} width={64} />}
            <Tooltip cursor={CURSOR} content={<ChartTooltip series={series} formatX={formatX} />} />
            {series.map((s, i) => (
              <Bar key={s.key} dataKey={s.key} name={s.label} stackId="stack" fill={seriesStyle(s, i).color} stroke="var(--color-surface)" strokeWidth={1} maxBarSize={56} {...chartAnimation(reduced)} />
            ))}
          </RBarChart>
        </ResponsiveContainer>
      )}
    </ChartFrame>
  );
}
