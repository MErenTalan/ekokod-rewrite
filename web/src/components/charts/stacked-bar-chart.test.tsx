import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { setMatchMedia } from '@/test/match-media';
import { renderWithProviders } from '@/test/render';

import { StackedBarChart } from './stacked-bar-chart';

const base = { title: 'Fatura bileşimi', description: 'Ağustos 2026', xLabel: 'Fatura', empty: { title: 'Veri yok', description: 'Fatura seçin.' } };
const series = [
  { key: 'energy', label: 'Enerji', kind: 'cost' as const, unit: 'TRY' as const },
  { key: 'dist', label: 'Dağıtım', kind: 'consumption' as const, unit: 'TRY' as const },
];

describe('StackedBarChart', () => {
  it('stacks segments with surface separators', () => {
    setMatchMedia((q) => q.includes('reduce')); // bar paths only exist once the grow animation has run
    const { container } = renderWithProviders(<StackedBarChart {...base} series={series} data={[{ x: 'Ağu', energy: '100', dist: '40' }]} layout="horizontal" />);
    setMatchMedia(() => false);
    const rects = container.querySelectorAll('.recharts-bar-rectangle path');
    expect(rects).toHaveLength(2);
    rects.forEach((r) => expect(r).toHaveAttribute('stroke', 'var(--color-surface)'));
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<StackedBarChart {...base} series={series} data={[{ x: 'Ağu', energy: '100', dist: '40' }]} />);
    await expectNoAxeViolations(container);
  });
});
