import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { monthly } from './_fixture';
import { YearlyChart } from './yearly-chart';

describe('YearlyChart', () => {
  it('has a data table equivalent with every month', async () => {
    const r = renderWithProviders(<YearlyChart monthly={monthly} />);
    await userEvent.click(r.getByRole('button', { name: 'Veri tablosunu göster' }));
    const table = r.getByRole('table', { name: 'Aylık tüketim ve üretim' });
    expect(table).toHaveTextContent('Ocak');
    expect(table).toHaveTextContent('Aralık');
  });
});
