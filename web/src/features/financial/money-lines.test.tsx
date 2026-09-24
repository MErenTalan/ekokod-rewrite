import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { coverageSuffix, MoneyLines } from './money-lines';

describe('MoneyLines', () => {
  it('one line per currency; currencies never add (R253)', () => {
    const r = renderWithProviders(
      <MoneyLines
        list={[
          { currency: 'TRY', amount: '11150' },
          { currency: 'EUR', amount: '120' },
        ]}
      />,
    );
    expect(r.getByText(/11\.150,00/)).toBeVisible();
    expect(r.getByText('120,00 EUR')).toBeVisible();
  });

  it('an empty list says "veri yok", not zero', () => {
    const r = renderWithProviders(<MoneyLines list={[]} />);
    expect(r.getByText('veri yok')).toBeVisible();
  });

  it('knows the Turkish possessive for the coverage line', () => {
    expect([1, 2, 3, 6, 9, 10, 12].map(coverageSuffix)).toEqual([
      'i',
      'si',
      'ü',
      'sı',
      'u',
      'u',
      'si',
    ]);
  });
});
