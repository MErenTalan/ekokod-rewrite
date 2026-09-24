import { formatCurrency, formatNumber, formatQuantity, type Unit } from '@/lib/format';
import type { ReportFigure, ReportMoney, ReportPriceRange } from '@/lib/api/types';

/** The words a figure needs, from the `reports.value` namespace. */
export type ValueLabels = {
  noData: string;
  notSelected: string;
  coverage: (withData: number, of: number) => string;
  perKwh: string;
};

/** A nullable figure: "veri yok" when missing, never 0 (R258), with coverage when partial. */
export function formatFigure(f: ReportFigure | undefined, unit: Unit, l: ValueLabels): string {
  if (!f) return l.noData;
  if (f.excluded) return l.notSelected;
  if (f.value === undefined || f.value === null) return l.noData;
  const shown = formatQuantity(f.value, unit, { maxFractionDigits: 2 });
  return f.with_data < f.of ? `${shown} (${l.coverage(f.with_data, f.of)})` : shown;
}

/** One line per currency; currencies are never added together (R270). */
export function formatMoneyList(list: ReportMoney[] | undefined, l: ValueLabels): string[] {
  if (!list || list.length === 0) return [l.noData];
  return list.map((m) => {
    const shown = m.currency === 'TRY' ? formatCurrency(m.value) : `${formatNumber(m.value, { minFractionDigits: 2, maxFractionDigits: 2 })} ${m.currency}`;
    return m.with_data < m.of ? `${shown} (${l.coverage(m.with_data, m.of)})` : shown;
  });
}

/** One price when the ends meet, a range otherwise, "veri yok" when unresolved (R257). */
export function formatRange(r: ReportPriceRange | undefined, l: ValueLabels): string {
  if (!r?.min) return l.noData;
  const price = (v: string) => formatNumber(v, { minFractionDigits: 4, maxFractionDigits: 4 });
  if (!r.max || price(r.min) === price(r.max)) return `${price(r.min)} ${l.perKwh}`;
  return `${price(r.min)} – ${price(r.max)} ${l.perKwh}`;
}

/** The direction a delta's sign means; null when there is no delta (R259). */
export function deltaDirection(pct: string | undefined | null): 'up' | 'down' | 'flat' | null {
  if (pct === undefined || pct === null || pct === '') return null;
  if (/^-?0*(\.0*)?$/.test(pct)) return 'flat';
  return pct.startsWith('-') ? 'down' : 'up';
}
