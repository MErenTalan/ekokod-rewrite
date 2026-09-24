import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { forecast } from './_fixture';
import { PredictionResult } from './prediction-result';

const rows = [
  { label: '00:00', median: '5.00', p10: '4.00', p90: '6.00' },
  { label: '01:00', median: '6.50', p10: '5.00', p90: '8.00' },
];

describe('PredictionResult (R384)', () => {
  it('shows the result as a chart and as a table with a total', () => {
    const { getByRole } = renderWithProviders(
      <PredictionResult title="Günlük tahmin" points={forecast.points} rows={rows} firstColumn="Saat" summary="Perşembe için toplam tahmin: 11,5 kWh" notStored />,
    );
    expect(getByRole('figure', { name: 'Günlük tahmin' })).toBeInTheDocument();
    const table = getByRole('table', { name: 'Günlük tahmin' });
    expect(table).toHaveTextContent('00:00');
    expect(table).toHaveTextContent('Toplam');
    expect(table).toHaveTextContent('11,5');
    expect(getByRole('status')).toHaveTextContent('Perşembe için toplam tahmin');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<PredictionResult title="Günlük tahmin" points={forecast.points} rows={rows} firstColumn="Saat" />);
    await expectNoAxeViolations(container);
  });
});
