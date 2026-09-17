'use client';

import { Bar, CartesianGrid, ComposedChart, Line, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';

import { formatNumber, unitSymbol } from '@/lib/format';

import { ChartFrame, type BaseChartProps } from './_chart-frame';
import { ChartTooltip } from './_chart-tooltip';
import { ACTIVE_DOT, AXIS, chartAnimation, CURSOR, GRID, seriesStyle, toPlotData, type ChartSeries } from './_theme';
import { usePrefersReducedMotion } from './use-reduced-motion';

export type ComboChartProps = Omit<BaseChartProps, 'series'> & { bars: ChartSeries[]; lines: ChartSeries[] };

const tick = (v: number) => formatNumber(v, { maxFractionDigits: 1 });
const axisLabel = (unit: ChartSeries['unit'], side: 'left' | 'right') => ({
  value: unitSymbol(unit),
  angle: side === 'left' ? -90 : 90,
  position: side === 'left' ? ('insideLeft' as const) : ('insideRight' as const),
  fill: 'var(--color-foreground-muted)',
  fontSize: 12,
});

/** Bars first, lines on top; lines get a right axis only when their unit differs (07 §5). */
export function ComboChart({ bars, lines, ...props }: ComboChartProps) {
  const reduced = usePrefersReducedMotion();
  const series = [...bars, ...lines];
  const { height = 320, formatX, title, description } = props;
  const leftUnit = (bars[0] ?? lines[0])?.unit ?? 'kWh';
  const rightUnit = lines.find((l) => l.unit !== leftUnit)?.unit;
  return (
    <ChartFrame {...props} series={series} barKeys={bars.map((s) => s.key)}>
      {(plot) => (
        <ResponsiveContainer width="100%" height={height} initialDimension={{ width: 640, height }}>
          <ComposedChart data={toPlotData(plot, series.map((s) => s.key))} margin={{ top: 8, right: rightUnit ? 4 : 16, bottom: 4, left: 4 }} accessibilityLayer title={title} desc={description}>
            <CartesianGrid {...GRID} vertical={false} />
            <XAxis dataKey="x" tickFormatter={formatX} {...AXIS} />
            <YAxis yAxisId="left" {...AXIS} width={64} tickFormatter={tick} label={axisLabel(leftUnit, 'left')} />
            {rightUnit ? <YAxis yAxisId="right" orientation="right" {...AXIS} width={64} tickFormatter={tick} label={axisLabel(rightUnit, 'right')} /> : null}
            <Tooltip cursor={CURSOR} content={<ChartTooltip series={series} formatX={formatX} />} />
            {bars.map((s, i) => (
              <Bar key={s.key} yAxisId="left" dataKey={s.key} name={s.label} fill={seriesStyle(s, i).color} radius={2} maxBarSize={40} {...chartAnimation(reduced)} />
            ))}
            {lines.map((s, i) => {
              const style = seriesStyle(s, bars.length + i);
              return (
                <Line
                  key={s.key}
                  yAxisId={rightUnit && s.unit !== leftUnit ? 'right' : 'left'}
                  dataKey={s.key}
                  name={s.label}
                  stroke={style.color}
                  strokeDasharray={style.dash}
                  strokeWidth={2.5}
                  dot={false}
                  activeDot={ACTIVE_DOT}
                  {...chartAnimation(reduced)}
                />
              );
            })}
          </ComposedChart>
        </ResponsiveContainer>
      )}
    </ChartFrame>
  );
}
