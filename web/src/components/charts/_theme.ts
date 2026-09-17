// The one place series look is decided (07 §5, plan D10/D11). Colours are token references, resolved by SVG in both themes.
import type { Unit } from '@/lib/format';

export type SeriesKind =
  | 'consumption'
  | 'generation'
  | 'reactiveInductive'
  | 'reactiveCapacitive'
  | 'cost'
  | 'revenue'
  | 'forecast'
  | 'current'
  | 'previous'
  | 'target';

const COLOR: Record<SeriesKind, string> = {
  consumption: 'var(--color-consumption)',
  generation: 'var(--color-generation)',
  reactiveInductive: 'var(--color-reactive-inductive)',
  reactiveCapacitive: 'var(--color-reactive-capacitive)',
  cost: 'var(--color-cost)',
  revenue: 'var(--color-revenue)',
  forecast: 'var(--color-forecast)',
  current: 'var(--color-brand)',
  previous: 'var(--color-foreground-subtle)',
  target: 'var(--color-foreground-muted)',
};

export const DASHES = ['0', '6 3', '2 3', '8 3 2 3', '1 3', '12 4'] as const;
export const MAX_SERIES = 6;
export const DOWNSAMPLE_THRESHOLD = 1000;
export type ChartSeries = { key: string; label: string; kind: SeriesKind; unit: Unit };
export type Datum = { x: string | number } & Record<string, number | string | null>;

export function seriesStyle(s: ChartSeries, index: number): { color: string; dash: string } {
  return { color: COLOR[s.kind], dash: s.kind === 'forecast' ? '6 4' : DASHES[index % DASHES.length] };
}

export function chartAnimation(reducedMotion: boolean) {
  return { isAnimationActive: !reducedMotion, animationDuration: 400, animationEasing: 'ease-out' as const };
}

/** Plot geometry only; tables and tooltips format the original decimal string (plan D11). */
export function toPlot(v: number | string | null | undefined): number | null {
  return v === null || v === undefined || v === '' ? null : Number(v);
}

export const RAW = '__raw';
/** Numeric copy of each datum for Recharts, with the original kept under RAW for tooltips. */
export function toPlotData(data: Datum[], keys: string[]): Record<string, unknown>[] {
  return data.map((d) => ({ ...d, ...Object.fromEntries(keys.map((k) => [k, toPlot(d[k])])), [RAW]: d }));
}

export const AXIS = { stroke: 'var(--color-border-strong)', tick: { fill: 'var(--color-foreground-muted)', fontSize: 12 } };
export const GRID = { stroke: 'var(--color-border)', strokeDasharray: '3 3' };
export const CURSOR = { fill: 'var(--color-surface-sunken)' }; // Recharts' default cursor (#ccc) ignores the theme (M-15)
export const ACTIVE_DOT = { stroke: 'var(--color-surface)', fill: 'var(--color-brand)' }; // default activeDot fill (#fff) ignores the theme (M-15)
export const LINE_CURSOR = { stroke: 'var(--color-border-strong)' };
