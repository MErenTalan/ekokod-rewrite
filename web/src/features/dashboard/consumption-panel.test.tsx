import { describe, expect, it, vi } from 'vitest';

import type { ConsumptionRow } from '@/lib/api/types';
import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { CHART_POINT_CAP, ConsumptionPanelView } from './consumption-panel';

const row = (i: number, active: number): ConsumptionRow =>
  ({
    period_start: `2026-03-${String((i % 28) + 1).padStart(2, '0')}T00:00:00+03:00`,
    period_end: `2026-03-${String((i % 28) + 2).padStart(2, '0')}T00:00:00+03:00`,
    active_import: String(active),
    reactive_inductive_import: String(active / 4),
    reactive_capacitive_import: String(active / 20),
    partial: false,
    source: 'load_profile',
    suspect_registers: [],
  }) as ConsumptionRow;

const view = (overrides: Partial<React.ComponentProps<typeof ConsumptionPanelView>> = {}) => (
  <ConsumptionPanelView
    rows={[row(0, 100), row(1, 200)]}
    previousYear={null}
    granularity="daily"
    range={{ from: '2026-03-01', to: '2026-03-31' }}
    today="2026-03-31"
    onApply={() => {}}
    show={{ active: true, inductive: true, capacitive: false }}
    onShowChange={() => {}}
    {...overrides}
  />
);

describe('ConsumptionPanelView', () => {
  it('caps the chart and says so, without hiding rows from the table', () => {
    const rows = Array.from({ length: 60 }, (_, i) => row(i, 100 + i));
    const r = renderWithProviders(view({ rows }));
    expect(r.getByText(`Grafik ilk ${CHART_POINT_CAP} noktayı gösteriyor (toplam 60).`)).toBeInTheDocument();
    expect(r.getByText('60 kayıt')).toBeInTheDocument();
  });

  it('draws only the series that are switched on', async () => {
    const onShowChange = vi.fn();
    const r = renderWithProviders(view({ onShowChange }));
    const legend = r.getAllByRole('list')[0];
    expect(legend).toHaveTextContent('Aktif');
    expect(legend).toHaveTextContent('Endüktif');
    expect(legend).not.toHaveTextContent('Kapasitif');
    await r.user.click(r.getByRole('switch', { name: 'Kapasitif' }));
    expect(onShowChange).toHaveBeenCalledWith({ active: true, inductive: true, capacitive: true });
  });

  it('shows the year-over-year bars only when a comparison was loaded', () => {
    const without = renderWithProviders(view());
    expect(without.getByText('Yıllık karşılaştırma, tek bir yıl içindeki aylık görünümde gösterilir.')).toBeInTheDocument();
    without.unmount();
    const withYoy = renderWithProviders(view({ previousYear: [row(0, 90), row(1, 180)], granularity: 'monthly' }));
    expect(withYoy.getAllByText('Yıllık karşılaştırma').length).toBeGreaterThan(0);
    // The legend appends the unit, so match the label itself.
    expect(withYoy.getAllByText(/Önceki yıl/).length).toBeGreaterThan(0);
  });

  it('applies the draft filters only when asked', async () => {
    const onApply = vi.fn();
    const r = renderWithProviders(view({ onApply }));
    await r.user.click(r.getByRole('button', { name: 'Uygula' }));
    expect(onApply).toHaveBeenCalledWith('daily', { from: '2026-03-01', to: '2026-03-31' });
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(view());
    await expectNoAxeViolations(r.container);
  });
});
