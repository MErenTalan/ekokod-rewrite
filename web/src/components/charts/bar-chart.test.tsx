import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { setMatchMedia } from '@/test/match-media';
import { renderWithProviders } from '@/test/render';

import { BarChart } from './bar-chart';

const base = { title: 'Kategoriye göre emisyon', description: '2026', xLabel: 'Kategori', empty: { title: 'Veri yok', description: 'Veri girin.' } };
const series = [{ key: 't', label: 'Emisyon', kind: 'cost' as const, unit: 'tCO2e' as const }];
const data = [
  { x: 'Doğalgaz', t: '12.5' },
  { x: 'Elektrik', t: '48.25' },
  { x: 'Araçlar', t: '7' },
];

describe('BarChart', () => {
  it('sort desc orders categories; reference line is labelled', async () => {
    const { container, getByText, getByRole, user } = renderWithProviders(
      <BarChart {...base} series={series} data={data} sort="desc" layout="horizontal" referenceLine={{ value: 20, label: 'Hedef 20 tCO₂e' }} />,
    );
    expect(getByText('Hedef 20 tCO₂e')).toBeInTheDocument();
    expect(container.querySelectorAll('.recharts-bar-rectangle').length).toBe(3);
    await user.click(getByRole('button', { name: 'Veri tablosunu göster' }));
    const firstRow = getByRole('table').querySelectorAll('tbody tr')[0];
    expect(firstRow).toHaveTextContent('Elektrik');
  });

  it('diverging colours by sign and draws a zero line', () => {
    const balance = [
      { x: 'Oca', net: '120' },
      { x: 'Şub', net: '-80' },
    ];
    setMatchMedia((q) => q.includes('reduce')); // bar paths only exist once the grow animation has run
    const { container } = renderWithProviders(
      <BarChart
        {...base}
        diverging
        series={[
          { key: 'net', label: 'Şebekeden çekiş', kind: 'consumption', unit: 'kWh' },
          { key: 'net', label: 'Şebekeye veriş', kind: 'generation', unit: 'kWh' },
        ]}
        data={balance}
      />,
    );
    setMatchMedia(() => false);
    const fills = [...container.querySelectorAll('.recharts-bar-rectangle path')].map((p) => p.getAttribute('fill'));
    expect(fills).toEqual(['var(--color-consumption)', 'var(--color-generation)']);
    expect(container.querySelector('.recharts-reference-line')).not.toBeNull();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<BarChart {...base} series={series} data={data} />);
    await expectNoAxeViolations(container);
  });
});
