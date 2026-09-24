import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { ChartTooltip } from './_chart-tooltip';
import { RAW, type ChartSeries } from './_theme';

const series: ChartSeries[] = [
  { key: 'c', label: 'Tüketim', kind: 'consumption', unit: 'kWh' },
  { key: 'g', label: 'Üretim', kind: 'generation', unit: 'kWh' },
  { key: 'cost', label: 'Maliyet', kind: 'cost', unit: 'TRY' },
];
const raw = { x: 'Ağu', c: '1234.5', g: null, cost: '9876.54' };
const payload = series.map((s) => ({ dataKey: s.key, payload: { ...raw, [RAW]: raw } }));

describe('ChartTooltip', () => {
  it('lists every series at x with units', () => {
    const { getAllByRole, getByText } = renderWithProviders(<ChartTooltip active payload={payload} label="Ağu" series={series} />);
    expect(getAllByRole('listitem')).toHaveLength(3);
    expect(getByText('1.234,5 kWh')).toBeInTheDocument();
    expect(getByText('—')).toBeInTheDocument();
    expect(getByText('₺9.876,54')).toBeInTheDocument();
    expect(getByText('Ağu')).toBeInTheDocument();
  });

  it('renders nothing when inactive', () => {
    const { container } = renderWithProviders(<ChartTooltip active={false} payload={payload} label="Ağu" series={series} />);
    expect(container.querySelector('ul')).toBeNull();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<ChartTooltip active payload={payload} label="Ağu" series={series} />);
    await expectNoAxeViolations(container);
  });
});
