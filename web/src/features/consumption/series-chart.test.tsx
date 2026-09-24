import { describe, expect, it, vi } from 'vitest';

import type { ConsumptionRow } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { SeriesChartView } from './series-chart';

const rows = [
  { period_start: '2026-03-14T00:00:00+03:00', active_import: '1000', reactive_inductive_import: '250', reactive_capacitive_import: '40' },
  { period_start: '2026-03-15T00:00:00+03:00', active_import: '1100', reactive_inductive_import: '260', reactive_capacitive_import: '45' },
] as ConsumptionRow[];

describe('SeriesChartView', () => {
  it('draws only the series that are switched on', async () => {
    const onShowChange = vi.fn();
    const r = renderWithProviders(
      <SeriesChartView
        rows={rows}
        granularity="daily"
        show={{ active: true, inductive: false, capacitive: false }}
        onShowChange={onShowChange}
      />,
    );
    const legend = r.getAllByRole('list')[0];
    expect(legend).toHaveTextContent('Aktif tüketim');
    expect(legend).not.toHaveTextContent('Endüktif tüketim');
    await r.user.click(r.getByRole('switch', { name: 'Endüktif tüketim' }));
    expect(onShowChange).toHaveBeenCalledWith({ active: true, inductive: true, capacitive: false });
  });

  it('shows an empty state instead of an empty axis frame', () => {
    const r = renderWithProviders(
      <SeriesChartView rows={[]} granularity="daily" show={{ active: true, inductive: false, capacitive: false }} onShowChange={() => {}} />,
    );
    expect(r.getByText('Seçilen dönem için veri yok')).toBeInTheDocument();
  });
});
