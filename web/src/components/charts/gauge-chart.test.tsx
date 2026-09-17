import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { GaugeChart } from './gauge-chart';

const base = {
  title: 'Endüktif oran',
  description: 'Ağustos 2026',
  label: 'Endüktif / aktif',
  min: 0,
  max: 30,
  unit: 'percent' as const,
  thresholds: [
    { value: 15, label: 'Uyarı %15', status: 'warning' as const },
    { value: 20, label: 'Ceza sınırı %20', status: 'danger' as const },
  ],
  empty: { title: 'Veri yok', description: 'Dönem seçin.' },
};

describe('GaugeChart', () => {
  it('thresholds are labelled in text; null value is no-data', () => {
    const { getByText, rerender } = renderWithProviders(<GaugeChart {...base} value="22.4" />);
    expect(getByText('Ceza sınırı %20')).toBeInTheDocument();
    expect(getByText('%22,4')).toBeInTheDocument();
    rerender(<GaugeChart {...base} value={null} />);
    expect(getByText('Veri yok')).toBeInTheDocument();
  });

  it('data table lists value and thresholds', async () => {
    const { getByRole, user } = renderWithProviders(<GaugeChart {...base} value="22.4" />);
    await user.click(getByRole('button', { name: 'Veri tablosunu göster' }));
    expect(getByRole('table')).toHaveTextContent('Endüktif / aktif');
    expect(getByRole('table')).toHaveTextContent('%20');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<GaugeChart {...base} value="12" />);
    await expectNoAxeViolations(container);
  });
});
