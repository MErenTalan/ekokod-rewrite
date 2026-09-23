import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { monthly } from './_fixture';
import { OffsetCard } from './offset-card';

describe('OffsetCard', () => {
  it('labels the split as computed netting', () => {
    const r = renderWithProviders(<OffsetCard figures={monthly.total} />);
    expect(r.getByRole('heading', { name: 'Mahsuplaşma (hesaplanan)' })).toBeVisible();
    expect(r.getByText('2.100 kWh')).toBeVisible();
  });

  it('without production it explains, never shows 0', () => {
    const r = renderWithProviders(<OffsetCard figures={monthly.items[2]} />);
    expect(r.getByText('Tüketim ya da üretim olmadan hesaplanamaz.')).toBeVisible();
    expect(r.queryByText('0 kWh')).toBeNull();
  });
});
