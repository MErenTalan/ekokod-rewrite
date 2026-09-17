import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { AreaChart } from './area-chart';

const base = { title: 'Günlük üretim', description: 'GES Sahası', xLabel: 'Gün', empty: { title: 'Veri yok', description: 'Santral seçin.' } };
const data = [1, 2, 3].map((d) => ({ x: `${d} Eyl`, gen: String(d * 120.5) }));

describe('AreaChart', () => {
  it('type rejects two series', () => {
    // @ts-expect-error — an area chart takes exactly one series (07 §5)
    const bad = <AreaChart {...base} data={data} series={[{ key: 'a', label: 'A', kind: 'generation', unit: 'kWh' }, { key: 'b', label: 'B', kind: 'generation', unit: 'kWh' }]} />;
    expect(bad).toBeTruthy();
    const { container } = renderWithProviders(<AreaChart {...base} data={data} series={[{ key: 'gen', label: 'Üretim', kind: 'generation', unit: 'kWh' }]} />);
    expect(container.querySelectorAll('.recharts-area-area')).toHaveLength(1);
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<AreaChart {...base} data={data} series={[{ key: 'gen', label: 'Üretim', kind: 'generation', unit: 'kWh' }]} />);
    await expectNoAxeViolations(container);
  });
});
