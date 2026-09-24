'use client';

import { Area, AreaChart as RAreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';

import { formatNumber, unitSymbol } from '@/lib/format';

import { ChartFrame, type BaseChartProps } from './_chart-frame';
import { ChartTooltip } from './_chart-tooltip';
import { ACTIVE_DOT, AXIS, chartAnimation, GRID, LINE_CURSOR, seriesStyle, toPlotData, type ChartSeries } from './_theme';
import { usePrefersReducedMotion } from './use-reduced-motion';

/** Area only for a single series (07 §5), enforced by the tuple type. */
export type AreaChartProps = Omit<BaseChartProps, 'series'> & { series: [ChartSeries] };

export function AreaChart(props: AreaChartProps) {
  const reduced = usePrefersReducedMotion();
  const { series, height = 320, formatX, title, description } = props;
  const [s] = series;
  const style = seriesStyle(s, 0);
  return (
    <ChartFrame {...props}>
      {(plot) => (
        <ResponsiveContainer width="100%" height={height} initialDimension={{ width: 640, height }}>
          <RAreaChart data={toPlotData(plot, [s.key])} margin={{ top: 8, right: 16, bottom: 4, left: 4 }} accessibilityLayer title={title} desc={description}>
            <CartesianGrid {...GRID} vertical={false} />
            <XAxis dataKey="x" tickFormatter={formatX} {...AXIS} />
            <YAxis
              {...AXIS}
              width={64}
              tickFormatter={(v: number) => formatNumber(v, { maxFractionDigits: 1 })}
              label={{ value: unitSymbol(s.unit), angle: -90, position: 'insideLeft', fill: 'var(--color-foreground-muted)', fontSize: 12 }}
            />
            <Tooltip cursor={LINE_CURSOR} content={<ChartTooltip series={series} formatX={formatX} />} />
            <Area dataKey={s.key} name={s.label} stroke={style.color} strokeWidth={2} fill={style.color} fillOpacity={0.18} activeDot={ACTIVE_DOT} connectNulls={false} {...chartAnimation(reduced)} />
          </RAreaChart>
        </ResponsiveContainer>
      )}
    </ChartFrame>
  );
}
