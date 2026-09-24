import { within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { ChartFrame } from './_chart-frame';
import type { ChartSeries, Datum } from './_theme';

const series: ChartSeries[] = [{ key: 'kwh', label: 'Tüketim', kind: 'consumption', unit: 'kWh' }];
const base = { title: 'Aylık tüketim', description: 'Merkez Bina, 2026', xLabel: 'Ay', empty: { title: 'Veri yok', description: 'Analizör seçin.' } };
const plot = (points: Datum[]) => <svg data-testid="plot" data-points={points.length} />;

describe('ChartFrame', () => {
  it('figure is named and described', () => {
    const { getByRole, getByText } = renderWithProviders(
      <ChartFrame {...base} series={series} data={[{ x: 'Oca', kwh: '1' }]}>
        {plot}
      </ChartFrame>,
    );
    const figure = getByRole('figure', { name: 'Aylık tüketim' });
    expect(figure.getAttribute('aria-describedby')).toBe(getByText('Merkez Bina, 2026').id);
  });

  it('data table toggles and shows exact values with units', async () => {
    const { getByRole, user } = renderWithProviders(
      <ChartFrame {...base} series={series} data={[{ x: 'Oca', kwh: '1234.567891' }]}>
        {plot}
      </ChartFrame>,
    );
    const toggle = getByRole('button', { name: 'Veri tablosunu göster' });
    await user.click(toggle);
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    const table = getByRole('table', { name: 'Aylık tüketim' });
    expect(within(table).getByRole('columnheader', { name: 'Tüketim (kWh)' })).toBeInTheDocument();
    expect(within(table).getByText('1.234,567891')).toBeInTheDocument();
    expect(getByRole('button', { name: 'Veri tablosunu gizle' })).toBeInTheDocument();
  });

  it('loading keeps the final height', () => {
    const { container, queryByTestId } = renderWithProviders(
      <ChartFrame {...base} series={series} data={[]} loading>
        {plot}
      </ChartFrame>,
    );
    expect((container.querySelector('[aria-hidden="true"].animate-pulse') as HTMLElement).style.height).toBe('320px');
    expect(queryByTestId('plot')).toBeNull();
  });

  it('empty state explains what is missing', () => {
    const { getByText, container } = renderWithProviders(
      <ChartFrame {...base} series={series} data={[]}>
        {plot}
      </ChartFrame>,
    );
    expect(getByText('Veri yok')).toBeInTheDocument();
    expect(container.querySelector('svg.recharts-surface, [data-testid="plot"]')).toBeNull();
  });

  it('more than six series renders an alert, not a chart', () => {
    const many = Array.from({ length: 7 }, (_, i): ChartSeries => ({ key: `s${i}`, label: `S${i}`, kind: 'consumption', unit: 'kWh' }));
    const { getByRole, queryByTestId } = renderWithProviders(
      <ChartFrame {...base} series={many} data={[{ x: 1, s0: '1' }]}>
        {plot}
      </ChartFrame>,
    );
    expect(within(getByRole('figure', { name: 'Aylık tüketim' })).getByRole('alert')).toHaveTextContent('en fazla 6 seri');
    expect(queryByTestId('plot')).toBeNull();
  });

  // Rendering a 3000-row table in jsdom takes a few seconds.
  it('over 1000 points downsamples and says so', { timeout: 30_000 }, async () => {
    const data = Array.from({ length: 3000 }, (_, i) => ({ x: i, kwh: String(i % 97) }));
    const { getByTestId, getByText, getByRole, user } = renderWithProviders(
      <ChartFrame {...base} series={series} data={data}>
        {plot}
      </ChartFrame>,
    );
    expect(getByTestId('plot')).toHaveAttribute('data-points', '1000');
    expect(getByText(/1\.000 \/ 3\.000/)).toBeInTheDocument();
    await user.click(getByRole('button', { name: 'Veri tablosunu göster' }));
    expect(within(getByRole('table')).getAllByRole('row')).toHaveLength(3001);
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(
      <ChartFrame {...base} series={series} data={[{ x: 'Oca', kwh: '1' }]} dataTableDefaultOpen>
        {plot}
      </ChartFrame>,
    );
    await expectNoAxeViolations(container);
  });
});
