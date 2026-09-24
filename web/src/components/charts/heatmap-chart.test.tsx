import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { HeatmapChart } from './heatmap-chart';

const base = {
  title: 'Saat × gün tüketimi',
  description: 'Merkez Bina, Ağustos 2026',
  xLabel: 'Saat',
  rows: ['Pzt', 'Sal'],
  columns: ['00', '01', '02'],
  values: [
    ['10', '20', null],
    ['30', '40.5', '50'],
  ],
  unit: 'kWh' as const,
  kind: 'consumption' as const,
  empty: { title: 'Veri yok', description: 'Analizör seçin.' },
};

describe('HeatmapChart', () => {
  it('null cells are distinguishable without colour', async () => {
    const { container, getByRole, user } = renderWithProviders(<HeatmapChart {...base} />);
    const cells = container.querySelectorAll('rect[data-cell]');
    expect(cells).toHaveLength(6);
    expect(cells[2].getAttribute('fill')).toMatch(/^url\(#/);
    await user.click(getByRole('button', { name: 'Veri tablosunu göster' }));
    expect(getByRole('table')).toHaveTextContent('Veri yok');
    expect(getByRole('table')).toHaveTextContent('40,5');
  });

  it('bucket legend shows ranges', () => {
    const { getByText } = renderWithProviders(<HeatmapChart {...base} />);
    expect(getByText('10 – 18')).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<HeatmapChart {...base} />);
    await expectNoAxeViolations(container);
  });
});
