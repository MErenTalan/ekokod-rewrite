'use client';

import { useTranslations } from 'next-intl';

import { formatNumber } from '@/lib/format';

import { seriesStyle, toPlot, type SeriesKind } from './_theme';

export type SparklineProps = { data: (string | null)[]; kind: SeriesKind; label: string; width?: number; height?: number };

/** A glyph beside a figure that is already text, so no frame or table (plan D12); gaps stay gaps. */
export function Sparkline({ data, kind, label, width = 120, height = 32 }: SparklineProps) {
  const t = useTranslations('charts');
  const present = data.filter((v): v is string => v !== null);
  const nums = present.map(Number);
  const min = Math.min(...nums);
  const max = Math.max(...nums);
  const pad = 2;
  const x = (i: number) => pad + (i * (width - 2 * pad)) / Math.max(1, data.length - 1);
  const y = (v: number) => (max === min ? height / 2 : pad + ((max - v) * (height - 2 * pad)) / (max - min));
  let d = '';
  let pen = false;
  data.forEach((v, i) => {
    const n = toPlot(v);
    if (n === null) {
      pen = false;
      return;
    }
    d += `${pen ? 'L' : 'M'}${x(i).toFixed(1)},${y(n).toFixed(1)}`;
    pen = true;
  });
  const summary =
    present.length === 0
      ? t('noData')
      : t('sparklineSummary', {
          first: formatNumber(present[0]),
          last: formatNumber(present[present.length - 1]),
          min: formatNumber(String(min)),
          max: formatNumber(String(max)),
        });
  return (
    <svg role="img" aria-label={`${label}: ${summary}`} width={width} height={height} viewBox={`0 0 ${width} ${height}`} className="shrink-0">
      <path d={d} fill="none" stroke={seriesStyle({ key: 's', label, kind, unit: 'kWh' }, 0).color} strokeWidth={1.75} strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  );
}
