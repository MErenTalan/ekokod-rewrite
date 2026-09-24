import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { ChartDataTable } from './_chart-data-table';

describe('ChartDataTable', () => {
  it('formats x and keeps exact decimals', () => {
    const { getByRole, getByText } = renderWithProviders(
      <ChartDataTable
        caption="Tüketim"
        xLabel="Saat"
        formatX={(x) => `${x}:00`}
        series={[{ key: 'kwh', label: 'Tüketim', kind: 'consumption', unit: 'kWh' }]}
        data={[{ x: 9, kwh: '12345678901234567.891' }, { x: 10, kwh: null }]}
      />,
    );
    expect(getByRole('table', { name: 'Tüketim' })).toBeInTheDocument();
    expect(getByText('9:00')).toBeInTheDocument();
    expect(getByText('12.345.678.901.234.567,891')).toHaveClass('type-data');
    expect(getByText('—')).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(
      <ChartDataTable caption="T" xLabel="X" series={[{ key: 'k', label: 'K', kind: 'cost', unit: 'TRY' }]} data={[{ x: 1, k: '2' }]} />,
    );
    await expectNoAxeViolations(container);
  });
});
