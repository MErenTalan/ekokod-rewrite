import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { setMatchMedia } from '@/test/match-media';
import { renderWithProviders } from '@/test/render';

import type { ChartSeries } from './_theme';
import { LineChart } from './line-chart';

const lineProps = vi.hoisted(() => [] as Record<string, unknown>[]);
vi.mock('recharts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('recharts')>();
  const Line = (props: Record<string, unknown>) => {
    lineProps.push(props);
    return <actual.Line {...(props as React.ComponentProps<typeof actual.Line>)} />;
  };
  return { ...actual, Line };
});

const series: ChartSeries[] = [
  { key: 'thisYear', label: '2026', kind: 'current', unit: 'kWh' },
  { key: 'lastYear', label: '2025', kind: 'previous', unit: 'kWh' },
];
const data = ['Oca', 'Şub', 'Mar', 'Nis'].map((x, i) => ({ x, thisYear: String(1000 + i * 50), lastYear: String(900 + i * 70) }));
const props = { title: 'Aylık tüketim', description: 'Merkez Bina', xLabel: 'Ay', series, data, empty: { title: 'Veri yok', description: 'Analizör seçin.' } };

describe('LineChart', () => {
  it('renders one path per series with the series dash', () => {
    // The draw animation owns stroke-dasharray until it ends, and jsdom never advances it.
    setMatchMedia((q) => q.includes('reduce'));
    const { container } = renderWithProviders(<LineChart {...props} />);
    setMatchMedia(() => false);
    const curves = container.querySelectorAll('.recharts-line-curve');
    expect(curves).toHaveLength(2);
    expect(curves[1]).toHaveAttribute('stroke-dasharray', '6 3');
  });

  it('animation follows prefers-reduced-motion', () => {
    lineProps.length = 0;
    setMatchMedia((q) => q.includes('reduce'));
    renderWithProviders(<LineChart {...props} />);
    expect(lineProps.length).toBeGreaterThan(0);
    expect(lineProps.every((p) => p.isAnimationActive === false)).toBe(true);
    setMatchMedia(() => false);
  });

  it('labels the value axis with the unit', () => {
    const { container } = renderWithProviders(<LineChart {...props} />);
    expect([...container.querySelectorAll('.recharts-label')].some((l) => l.textContent === 'kWh')).toBe(true);
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<LineChart {...props} />);
    await expectNoAxeViolations(container);
  });
});
