import type { CSSProperties } from 'react';

/** Customiser state (plan D15), persisted in a cookie so SSR renders the right theme and layout without a flash. */
export const UI_COOKIE = 'ekokod_ui';
export type UiPreferences = {
  theme: 'light' | 'dark' | 'system'; layout: 'vertical' | 'horizontal'; container: 'full' | 'boxed';
  sidebar: 'expanded' | 'collapsed'; card: 'border' | 'shadow'; radiusScale: number;
};
export const defaultUiPreferences: UiPreferences = {
  theme: 'system', layout: 'vertical', container: 'full', sidebar: 'expanded', card: 'border', radiusScale: 1,
};
const pick = <T extends string>(v: unknown, allowed: readonly T[], fallback: T): T =>
  typeof v === 'string' && (allowed as readonly string[]).includes(v) ? (v as T) : fallback;
export function parseUiPreferences(raw: string | undefined): UiPreferences {
  let o: Record<string, unknown> = {};
  try { o = raw ? (JSON.parse(decodeURIComponent(raw)) as Record<string, unknown>) : {}; } catch { o = {}; }
  const d = defaultUiPreferences;
  const r = typeof o.radiusScale === 'number' && Number.isFinite(o.radiusScale) ? o.radiusScale : d.radiusScale;
  return {
    theme: pick(o.theme, ['light', 'dark', 'system'], d.theme),
    layout: pick(o.layout, ['vertical', 'horizontal'], d.layout),
    container: pick(o.container, ['full', 'boxed'], d.container),
    sidebar: pick(o.sidebar, ['expanded', 'collapsed'], d.sidebar),
    card: pick(o.card, ['border', 'shadow'], d.card),
    radiusScale: Math.min(1.5, Math.max(0.5, Math.round(r * 4) / 4)),
  };
}
export function serializeUiPreferences(p: UiPreferences): string { return encodeURIComponent(JSON.stringify(p)); }
export function htmlAttributes(p: UiPreferences) {
  return { 'data-theme': p.theme, 'data-layout': p.layout, 'data-container': p.container, 'data-sidebar': p.sidebar,
    'data-card': p.card, style: { '--radius-scale': String(p.radiusScale) } as CSSProperties };
}
