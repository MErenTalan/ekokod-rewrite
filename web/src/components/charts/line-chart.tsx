'use client';

import { Area, CartesianGrid, ComposedChart, LabelList, Line, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { useTranslations } from 'next-intl';

import { formatNumber, unitSymbol } from '@/lib/format';

import { ChartFrame, type BaseChartProps } from './_chart-frame';
import { ChartTooltip } from './_chart-tooltip';
import { ACTIVE_DOT, AXIS, chartAnimation, GRID, LINE_CURSOR, seriesStyle, toPlot, toPlotData } from './_theme';
import { usePrefersReducedMotion } from './use-reduced-motion';

export type LineChartProps = BaseChartProps & {
  /** Shaded p10–p90 style band, always neutral (07 §5). */
  band?: { lowerKey: string; upperKey: string; label: string };
  directLabels?: boolean;
};

export function LineChart({ band, directLabels = true, ...props }: LineChartProps) {
  const t = useTranslations('charts');
  const reduced = usePrefersReducedMotion();
  const { series, height = 320, formatX, title, description } = props;
  const keys = [...series.map((s) => s.key), ...(band ? [band.lowerKey, band.upperKey] : [])];
  return (
    <ChartFrame
      {...props}
      legendExtra={band ? [{ label: band.label || t('forecastBand'), color: 'var(--color-forecast)', dash: '6 4', swatch: 'area' }] : undefined}
    >
      {(plot) => (
        <ResponsiveContainer width="100%" height={height} initialDimension={{ width: 640, height }}>
          <ComposedChart data={toPlotData(plot, keys)} margin={{ top: 8, right: directLabels ? 72 : 16, bottom: 4, left: 4 }} accessibilityLayer title={title} desc={description}>
            <CartesianGrid {...GRID} vertical={false} />
            <XAxis dataKey="x" tickFormatter={formatX} {...AXIS} />
            <YAxis
              {...AXIS}
              width={64}
              tickFormatter={(v: number) => formatNumber(v, { maxFractionDigits: 1 })}
              label={{ value: unitSymbol(series[0]?.unit ?? 'kWh'), angle: -90, position: 'insideLeft', fill: 'var(--color-foreground-muted)', fontSize: 12 }}
            />
            <Tooltip cursor={LINE_CURSOR} content={<ChartTooltip series={series} formatX={formatX} />} />
            {band ? (
              <Area
                dataKey={(d: Record<string, unknown>) => [toPlot(d[band.lowerKey] as number), toPlot(d[band.upperKey] as number)]}
                name={band.label}
                stroke="var(--color-forecast)"
                strokeDasharray="6 4"
                fill="var(--color-forecast)"
                fillOpacity={0.15}
                activeDot={false}
                {...chartAnimation(reduced)}
              />
            ) : null}
            {series.map((s, i) => {
              const style = seriesStyle(s, i);
              return (
                <Line
                  key={s.key}
                  dataKey={s.key}
                  name={s.label}
                  stroke={style.color}
                  strokeDasharray={style.dash}
                  strokeWidth={2}
                  dot={false}
                  activeDot={ACTIVE_DOT}
                  connectNulls={false}
                  {...chartAnimation(reduced)}
                >
                  {directLabels ? (
                    <LabelList
                      dataKey={s.key}
                      content={(p) =>
                        p.index === plot.length - 1 && p.value != null ? (
                          <text x={Number(p.x) + 6} y={Number(p.y)} dy={4} fill="var(--color-foreground-muted)" fontSize={12}>
                            {s.label}
                          </text>
                        ) : null
                      }
                    />
                  ) : null}
                </Line>
              );
            })}
          </ComposedChart>
        </ResponsiveContainer>
      )}
    </ChartFrame>
  );
}
