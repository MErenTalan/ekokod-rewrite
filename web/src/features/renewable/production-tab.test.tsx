import { describe, expect, it, vi } from 'vitest';

import type { ConsumptionRow } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { ProductionTabView } from './production-tab';

const rows = [
  {
    period_start: '2026-09-18T00:00:00+03:00',
    period_end: '2026-09-19T00:00:00+03:00',
    active_export: '180.5',
    reactive_inductive_export: '2',
    partial: false,
    source: 'meter',
  },
  {
    period_start: '2026-09-19T00:00:00+03:00',
    period_end: '2026-09-20T00:00:00+03:00',
    active_export: undefined,
    partial: false,
    source: 'meter',
  },
] as ConsumptionRow[];

describe('ProductionTabView', () => {
  it('lists generation per period from the export registers, missing as "veri yok"', () => {
    const r = renderWithProviders(
      <ProductionTabView
        rows={rows}
        granularity="daily"
        show={{ active: true, inductive: true, capacitive: false }}
        onShowChange={() => {}}
      />,
    );
    const table = r.getByRole('table', { name: 'Üretim tablosu' });
    expect(table).toHaveTextContent('180,5');
    expect(table).toHaveTextContent('veri yok');
  });

  it('toggles a series', () => {
    const onShowChange = vi.fn();
    const r = renderWithProviders(
      <ProductionTabView
        rows={rows}
        granularity="daily"
        show={{ active: true, inductive: true, capacitive: false }}
        onShowChange={onShowChange}
      />,
    );
    r.getByRole('switch', { name: 'Kapasitif (kVArh)' }).click();
    expect(onShowChange).toHaveBeenCalledWith({ active: true, inductive: true, capacitive: true });
  });
});
