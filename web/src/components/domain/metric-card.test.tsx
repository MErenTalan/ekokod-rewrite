import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { MetricCard } from './metric-card';

describe('MetricCard', () => {
  it('delta colour follows sentiment, not direction', () => {
    const { container, getByText } = renderWithProviders(
      <MetricCard label="Tüketim" value="182345.1" unit="kWh" delta={{ value: '12.5', direction: 'up', sentiment: 'bad', comparisonLabel: 'geçen aya göre' }} />,
    );
    const delta = container.querySelector('[data-delta]')!;
    expect(delta).toHaveClass('text-danger');
    expect(delta.querySelector('svg.lucide-arrow-up')).not.toBeNull();
    expect(delta).toHaveTextContent('+%12,5');
    expect(getByText('geçen aya göre')).toBeInTheDocument();
    expect(getByText('182.345,1')).toHaveClass('type-metric');
  });

  it('a falling cost is good news', () => {
    const { container } = renderWithProviders(
      <MetricCard label="Maliyet" value="412908.2" unit="TRY" delta={{ value: '-3.2', direction: 'down', sentiment: 'good', comparisonLabel: 'geçen yıla göre' }} />,
    );
    expect(container.querySelector('[data-delta]')).toHaveClass('text-success');
    expect(container.querySelector('[data-delta]')).toHaveTextContent('−%3,2');
    expect(container).toHaveTextContent('₺412.908,20');
  });

  it('null value is an em dash and quality badge shows', () => {
    const { getByText } = renderWithProviders(<MetricCard label="Tüketim" value={null} unit="kWh" quality={{ state: 'estimated', reason: 'Tahmin' }} />);
    expect(getByText('—')).toBeInTheDocument();
    expect(getByText('Tahmini')).toBeInTheDocument();
  });

  it('loading shows a skeleton instead of a number', () => {
    const { container, queryByText } = renderWithProviders(<MetricCard label="Tüketim" value="1" unit="kWh" loading />);
    expect(container.querySelector('.animate-pulse')).not.toBeNull();
    expect(queryByText('1')).toBeNull();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<MetricCard label="Tüketim" value="1" unit="kWh" delta={{ value: '0', direction: 'flat', sentiment: 'neutral', comparisonLabel: 'geçen ay' }} />);
    await expectNoAxeViolations(container);
  });
});
