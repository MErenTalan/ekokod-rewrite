import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { overview } from './_fixture';
import { SummaryCardsView } from './summary-cards';

describe('SummaryCardsView', () => {
  it('shows the range totals with units', () => {
    const r = renderWithProviders(<SummaryCardsView data={overview} />);
    expect(r.getByText('Aktif üretim')).toBeVisible();
    expect(r.getByText('1.250,5')).toBeVisible();
    expect(r.getByText('12,25')).toBeVisible();
  });

  it('a range without generation says "veri yok", not 0', () => {
    const r = renderWithProviders(
      <SummaryCardsView
        data={{
          unavailable: { active_generation_kwh: 'no_data', average_generation_kwh: 'no_data' },
        }}
      />,
    );
    expect(r.getAllByText('veri yok')).toHaveLength(4);
    expect(r.queryByText('0')).toBeNull();
  });
});
