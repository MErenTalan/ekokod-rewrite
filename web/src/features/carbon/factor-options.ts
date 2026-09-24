import type { EmissionFactorView } from '@/lib/api/types';

export type Option = { value: string; label: string };

const usable = (f: EmissionFactorView) => f.status == null || f.status === 'active';
const humanise = (key: string) => key.replaceAll('_', ' ');

/**
 * The sub-category's factors as one searchable list (Q-F12): legacy labels
 * repeat across path leaves, so a repeated label carries its leaf.
 */
export function factorOptions(factors: EmissionFactorView[]): Option[] {
  const list = factors.filter(usable);
  const count = new Map<string, number>();
  for (const f of list) count.set(f.label, (count.get(f.label) ?? 0) + 1);
  return list
    .map((f) => {
      const leaf = f.category_path.at(-1);
      return { value: f.key, label: (count.get(f.label) ?? 0) > 1 && leaf ? `${f.label} (${humanise(leaf)})` : f.label };
    })
    .sort((a, b) => a.label.localeCompare(b.label, 'tr'));
}

/** The factor's base unit first (multiplier 1), then its other conversions (R305). */
export function unitOptions(factor: EmissionFactorView): Option[] {
  const base = factor.conversions.find((c) => c.unit === factor.base_unit);
  const out: Option[] = [{ value: factor.base_unit, label: base?.label ?? factor.base_unit }];
  for (const c of factor.conversions) {
    if (!out.some((o) => o.value === c.unit)) out.push({ value: c.unit, label: c.label });
  }
  return out;
}

/** The record keeps which leaf of the catalogue it used, within R305's detail limit. */
export function pathDetail(factor: EmissionFactorView): Record<string, string> {
  const path = factor.category_path.join(' > ');
  return path ? { path: path.slice(0, 100) } : {};
}
