'use client';

import { useTranslations } from 'next-intl';

import { unitSymbol } from '@/lib/format';

import { seriesStyle, type ChartSeries } from './_theme';

export type LegendExtra = { label: string; color: string; dash?: string; swatch?: 'line' | 'area' | 'bar' };

/** Colour + dash sample + text label with unit: series are never told apart by colour alone (07 §5). */
/** `barKeys`: series drawn as bars get a solid block, since bars carry no dash pattern. */
export function ChartLegend({ series, extra = [], barKeys = [] }: { series: ChartSeries[]; extra?: LegendExtra[]; barKeys?: string[] }) {
  const t = useTranslations('charts');
  const items: LegendExtra[] = [
    ...series.map((s, i) => {
      const style = seriesStyle(s, i);
      const bar = barKeys.includes(s.key);
      return { label: t('seriesWithUnit', { label: s.label, unit: unitSymbol(s.unit) }), color: style.color, dash: bar ? undefined : style.dash, swatch: bar ? ('bar' as const) : ('line' as const) };
    }),
    ...extra,
  ];
  return (
    <ul aria-label={t('legend')} className="flex flex-wrap gap-x-4 gap-y-1 text-foreground-muted type-small">
      {items.map((item) => (
        <li key={item.label} className="flex items-center gap-1.5">
          <svg aria-hidden width="24" height="10" className="shrink-0">
            {item.swatch === 'bar' ? (
              <rect x="6" y="0" width="12" height="10" rx="2" fill={item.color} />
            ) : item.swatch === 'area' ? (
              <rect x="0" y="1" width="24" height="8" fill={item.color} fillOpacity={0.15} stroke={item.color} strokeDasharray={item.dash} />
            ) : (
              <line x1="0" y1="5" x2="24" y2="5" stroke={item.color} strokeWidth="2.5" strokeDasharray={item.dash} />
            )}
          </svg>
          <span>{item.label}</span>
        </li>
      ))}
    </ul>
  );
}
