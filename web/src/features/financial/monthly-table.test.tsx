import { within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { monthly } from './_fixture';
import { MonthlyTable } from './monthly-table';

describe('MonthlyTable', () => {
  it('twelve months and the total row; months without data say so', () => {
    const r = renderWithProviders(<MonthlyTable monthly={monthly} />);
    const table = r.getByRole('table', { name: 'Aylık finansal tablo' });
    const rows = within(table).getAllByRole('row');
    expect(rows).toHaveLength(1 + 12 + 1);
    expect(within(rows[13]).getByText('Yıl toplamı')).toBeVisible();
    expect(within(rows[13]).getByText('3.600')).toBeVisible();
    expect(within(rows[4]).getAllByText('veri yok').length).toBeGreaterThan(0);
  });

  it('prints each currency on its own line and marks partial revenue', () => {
    const r = renderWithProviders(<MonthlyTable monthly={monthly} />);
    const feb = within(r.getByRole('table', { name: 'Aylık finansal tablo' })).getAllByRole(
      'row',
    )[2];
    expect(within(feb).getAllByText('120,00 EUR')).toHaveLength(2);
    expect(within(feb).getByText('eksik')).toBeVisible();
  });
});
