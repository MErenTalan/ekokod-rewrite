// Every foreground/background token pair the UI uses, with its WCAG minimum (07 §2.4, §9; plan D3).
import type { Pair } from './lib/contrast.ts';

export type { Pair };
const TEXT = 4.5, UI = 3;
const surfaces = ['background', 'surface', 'surface-raised', 'surface-sunken'];
export const pairs: Pair[] = [
  ...['foreground', 'foreground-muted', 'foreground-subtle'].flatMap((f) => surfaces.map((b) => [f, b, TEXT] as const)),
  ['on-primary', 'primary', TEXT], ['on-primary', 'primary-hover', TEXT], ['primary', 'background', TEXT],
  ['primary', 'surface', TEXT], ['primary', 'primary-subtle', TEXT], ['foreground', 'primary-subtle', TEXT],
  ['on-danger', 'danger', TEXT],
  ...['success', 'warning', 'danger', 'info'].flatMap((s) => [
    [s, `${s}-subtle`, TEXT], [s, 'surface', TEXT], [s, 'background', TEXT], ['foreground', `${s}-subtle`, TEXT]] as const),
  ...['ring', 'border-control', 'brand', 'consumption', 'generation', 'reactive-inductive', 'reactive-capacitive',
    'cost', 'revenue', 'forecast'].flatMap((g) => ['background', 'surface', 'surface-raised'].map((b) => [g, b, UI] as const)),
];
export const exempt: Record<string, string> = {
  border: 'decorative divider; never a component boundary (WCAG 1.4.11 n/a)',
  'border-strong': 'decorative divider; controls use border-control',
  overlay: 'translucent scrim; content above it is on surface tokens',
  'card-edge': 'decorative card edge in shadow mode (transparent in light)',
};
