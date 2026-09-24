import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { balance } from './_fixture';
import { EnergyBalanceCard } from './energy-balance-card';

describe('EnergyBalanceCard', () => {
  it('charts consumption, generation, import and export, and says the battery filter is unavailable', () => {
    const r = renderWithProviders(<EnergyBalanceCard data={balance} granularity="daily" />);
    expect(r.getByRole('heading', { name: 'Enerji dengesi' })).toBeVisible();
    expect(r.getByText('Batarya filtresi kullanılamıyor: batarya ölçümü yok.')).toBeVisible();
  });
});
