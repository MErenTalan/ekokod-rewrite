import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { ChartLegend } from './_chart-legend';
import type { ChartSeries } from './_theme';

const series: ChartSeries[] = [
  { key: 'c', label: 'Tüketim', kind: 'consumption', unit: 'kWh' },
  { key: 'g', label: 'Üretim', kind: 'generation', unit: 'kWh' },
];

describe('ChartLegend', () => {
  it('legend shows label, unit and dash sample', () => {
    const { getByText } = renderWithProviders(<ChartLegend series={series} />);
    const item = getByText('Üretim (kWh)').closest('li')!;
    expect(item.querySelector('line')).toHaveAttribute('stroke-dasharray', '6 3');
    expect(item.querySelector('svg')).toHaveAttribute('aria-hidden', 'true');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<ChartLegend series={series} extra={[{ label: 'Hedef', dash: '4 4', color: 'var(--color-foreground-muted)' }]} />);
    await expectNoAxeViolations(container);
  });
});
