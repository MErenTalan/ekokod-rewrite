import { describe, expect, it, vi } from 'vitest';

import type { LoadProfileStatistics } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { DetailsTabView } from './details-tab';

const statistics = {
  config: { weekend_days: [0, 6], weekend_source: 'company', vacations: 1 },
  statistics: {
    weekday: { max: '42.5', min: '10.25', hour_of_max: 14, mean: '25.125', stddev: '8.3', range: '32.25', load_factor: '0.5912' },
    weekend: { max: '20', min: '5', hour_of_max: 11, mean: '12', stddev: '4', range: '15', load_factor: '0.6' },
  },
} as unknown as LoadProfileStatistics;

describe('DetailsTabView', () => {
  it('shows each profile with its statistics', () => {
    const r = renderWithProviders(<DetailsTabView statistics={statistics} onExport={() => {}} />);
    const row = r.getByRole('row', { name: /Hafta içi/ });
    expect(row).toHaveTextContent('42,5');
    expect(row).toHaveTextContent('10,25');
    expect(row).toHaveTextContent('14:00');
    expect(row).toHaveTextContent('25,125');
    expect(row).toHaveTextContent('%59,12');
  });

  it('exports the hourly matrix', async () => {
    const onExport = vi.fn();
    const r = renderWithProviders(<DetailsTabView statistics={statistics} onExport={onExport} />);
    await r.user.click(r.getByRole('button', { name: 'Dışa aktar' }));
    await r.user.click(await r.findByRole('menuitem', { name: 'Excel' }));
    expect(onExport).toHaveBeenCalledWith('excel');
  });

  it('explains an empty table', () => {
    const r = renderWithProviders(<DetailsTabView statistics={null} onExport={() => {}} />);
    expect(r.getByText('İstatistik yok')).toBeInTheDocument();
  });
});
