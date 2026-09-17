import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { setMatchMedia } from '@/test/match-media';
import { renderWithProviders } from '@/test/render';

import { ComboChart } from './combo-chart';

const base = { title: 'Tüketim ve üretim', description: 'Merkez Bina', xLabel: 'Ay', empty: { title: 'Veri yok', description: 'Bina seçin.' } };
const bars = [{ key: 'c', label: 'Tüketim', kind: 'consumption' as const, unit: 'kWh' as const }];
const lines = [{ key: 'g', label: 'Üretim', kind: 'generation' as const, unit: 'kWh' as const }];
const data = ['Oca', 'Şub', 'Mar'].map((x, i) => ({ x, c: String(900 + i * 10), g: String(300 + i * 40) }));

describe('ComboChart', () => {
  it('bars and lines both render', () => {
    setMatchMedia((q) => q.includes('reduce'));
    const { container } = renderWithProviders(<ComboChart {...base} bars={bars} lines={lines} data={data} />);
    setMatchMedia(() => false);
    expect(container.querySelectorAll('.recharts-bar-rectangle').length).toBeGreaterThan(0);
    expect(container.querySelectorAll('.recharts-line-curve')).toHaveLength(lines.length);
  });

  it('more than six combined series is refused', () => {
    const many = Array.from({ length: 4 }, (_, i) => ({ key: `l${i}`, label: `L${i}`, kind: 'generation' as const, unit: 'kWh' as const }));
    const { getByRole } = renderWithProviders(<ComboChart {...base} bars={[...bars, ...bars.map((b) => ({ ...b, key: 'c2' })), ...bars.map((b) => ({ ...b, key: 'c3' }))]} lines={many} data={data} />);
    expect(getByRole('alert')).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<ComboChart {...base} bars={bars} lines={[{ ...lines[0], unit: 'TRY' }]} data={data} />);
    await expectNoAxeViolations(container);
  });
});
