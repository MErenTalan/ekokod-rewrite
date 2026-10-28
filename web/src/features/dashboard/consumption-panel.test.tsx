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
  it('caps the chart at 50 points and says so, without hiding rows from the table', async () => {
    const rows = Array.from({ length: 60 }, (_, i) => row(i, 100 + i));
    const r = renderWithProviders(view({ rows }));
    expect(r.getByText(`Grafik ilk ${CHART_POINT_CAP} noktayı gösteriyor (toplam 60).`)).toBeInTheDocument();
    expect(r.getByText('60 kayıt')).toBeInTheDocument();

    // The chart's own data table is what it plotted: it must stop at the cap.
    await r.user.click(r.getAllByRole('button', { name: 'Veri tablosunu göster' })[0]);
    const chartTable = r.getByRole('table', { name: 'Tüketim eğilimi' });
    expect(chartTable.querySelectorAll('tbody tr')).toHaveLength(CHART_POINT_CAP);
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

  it('asks for a building instead of claiming there is no data when none is selected', () => {
    const { getAllByText, queryByText } = renderWithProviders(view({ rows: [], needsBuilding: true }));
    expect(getAllByText('Tüketimi görmek için bir bina seçin').length).toBeGreaterThan(0);
    expect(queryByText('Seçilen dönem için veri yok')).toBeNull();
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

  // F29 smoke: last year's data began in September and was drawn under January.
  it('pairs last year by calendar month when last year starts mid-year', async () => {
    const month = (m: string, active: number) => ({ ...row(0, active), period_start: `${m}-01T00:00:00+03:00` });
    const r = renderWithProviders(view({
      rows: [month('2026-01', 100), month('2026-02', 200), month('2026-03', 300)],
      previousYear: [month('2025-03', 777)],
      granularity: 'monthly',
    }));
    await r.user.click(r.getAllByRole('button', { name: 'Veri tablosunu göster' })[1]);
    const cells = [...r.getByRole('table', { name: 'Yıllık karşılaştırma' }).querySelectorAll('tbody tr')]
      .map((tr) => [...tr.querySelectorAll('td, th')].map((c) => c.textContent));
    expect(cells[0][2]).not.toContain('777');
    expect(cells[2][2]).toContain('777');
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
