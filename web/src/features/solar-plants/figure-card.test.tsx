import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { FigureCard } from './figure-card';

describe('FigureCard', () => {
  it('shows a value with its unit', () => {
    const r = renderWithProviders(<FigureCard label="Anlık güç" value="125.4" unit="kW" />);
    expect(r.getByText('125,4')).toBeVisible();
    expect(r.getByText('kW')).toBeVisible();
  });

  it('says "veri yok" for a missing value, never a dash or zero', () => {
    const r = renderWithProviders(<FigureCard label="Bu ayki üretim" value={undefined} unit="kWh" />);
    expect(r.getByText('veri yok')).toBeVisible();
    expect(r.queryByText('—')).toBeNull();
  });
});
