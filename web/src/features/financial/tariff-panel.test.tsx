import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { summary } from './_fixture';
import { TariffPanel } from './tariff-panel';

describe('TariffPanel', () => {
  it('lists each building and plant with its prices', () => {
    const r = renderWithProviders(<TariffPanel tariffs={summary.tariffs} />);
    expect(r.getByText('A1 Fabrika')).toBeVisible();
    expect(r.getByText(/T2 \(puant\)/)).toBeVisible();
    expect(r.getByText('Konya GES')).toBeVisible();
  });

  it('says explicitly when no purchase or sale tariff is in force', () => {
    const r = renderWithProviders(
      <TariffPanel
        tariffs={{ purchase: [], sale: [], purchase_missing: true, sale_missing: true }}
      />,
    );
    expect(r.getByText('Hiçbir binada yürürlükte alış tarifesi yok.')).toBeVisible();
    expect(r.getByText('Hiçbir santralde yürürlükte satış tarifesi yok.')).toBeVisible();
  });
});
