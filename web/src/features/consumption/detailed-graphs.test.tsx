import { describe, expect, it, vi } from 'vitest';

import type { ConsumptionGrouped } from '@/lib/api/types';
import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { DetailedGraphsView } from './detailed-graphs';

const group = (key: string, active: string) => ({
  key,
  days: 5,
  active_import: active,
  reactive_inductive_import: '100',
  reactive_capacitive_import: '20',
  partial: false,
});

const grouped = (groupBy: string, keys: [string, string][], withPrevious = false): ConsumptionGrouped => ({
  group_by: groupBy,
  current: {
    from: '2026-03-01',
    to: '2026-03-31',
    groups: keys.map(([key, value]) => group(key, value)),
    statistics: {
      total: '3000',
      average: '1500',
      peak: { key: keys[0][0], value: keys[0][1] },
      valley: { key: keys[1]?.[0] ?? keys[0][0], value: keys[1]?.[1] ?? keys[0][1] },
    },
  },
  ...(withPrevious
    ? {
        previous: {
          from: '2026-01-29',
          to: '2026-02-28',
          groups: keys.map(([key, value]) => group(key, String(Number(value) * 0.8))),
          statistics: { total: '2400', average: '1200' },
        },
      }
    : {}),
});

const view = (overrides: Partial<React.ComponentProps<typeof DetailedGraphsView>> = {}) => (
  <DetailedGraphsView
    data={grouped('day_type', [['weekday', '2000'], ['weekend', '1000']])}
    groupBy="day_type"
    onGroupByChange={() => {}}
    compare={false}
    onCompareChange={() => {}}
    show={{ active: true, inductive: false, capacitive: false }}
    onShowChange={() => {}}
    {...overrides}
  />
);

describe('DetailedGraphsView', () => {
  it('labels day types, weeks and seasons the way people read them', async () => {
    const dayType = renderWithProviders(view());
    await dayType.user.click(dayType.getAllByRole('button', { name: 'Veri tablosunu göster' })[0]);
    expect(dayType.getByRole('table', { name: 'Gruplanmış tüketim' })).toHaveTextContent('Hafta içi');
    expect(dayType.getByRole('table', { name: 'Gruplanmış tüketim' })).toHaveTextContent('Hafta sonu');
    dayType.unmount();

    const week = renderWithProviders(
      view({ data: grouped('week', [['2026-03-02', '900'], ['2026-03-09', '1100']]), groupBy: 'week' }),
    );
    await week.user.click(week.getAllByRole('button', { name: 'Veri tablosunu göster' })[0]);
    expect(week.getByRole('table', { name: 'Gruplanmış tüketim' })).toHaveTextContent('Hafta 2 Mar 2026');
    week.unmount();

    const season = renderWithProviders(
      view({ data: grouped('season_day_type', [['summer-2026-weekend', '500'], ['winter-2025-weekday', '700']]), groupBy: 'season_day_type' }),
    );
    await season.user.click(season.getAllByRole('button', { name: 'Veri tablosunu göster' })[0]);
    expect(season.getByRole('table', { name: 'Gruplanmış tüketim' })).toHaveTextContent('Yaz 2026 · Hafta sonu');
  });

  it('shows the comparison chart only when asked', () => {
    const off = renderWithProviders(view());
    expect(off.queryByText('Dönem karşılaştırması')).toBeNull();
    off.unmount();
    const on = renderWithProviders(
      view({ data: grouped('day_type', [['weekday', '2000'], ['weekend', '1000']], true), compare: true }),
    );
    expect(on.getAllByText('Dönem karşılaştırması').length).toBeGreaterThan(0);
    expect(on.getAllByText(/Önceki dönem/).length).toBeGreaterThan(0);
  });

  it('reports the statistics of the current period', () => {
    const r = renderWithProviders(view());
    // The axis also prints numbers, so read the tiles themselves.
    const tile = (label: string) => r.getByText(label).parentElement!;
    expect(tile('Toplam')).toHaveTextContent('3.000');
    expect(tile('Ortalama')).toHaveTextContent('1.500');
    expect(tile('Tepe')).toHaveTextContent('Hafta içi');
    expect(tile('Dip')).toHaveTextContent('Hafta sonu');
  });

  it('reports a grouping change', async () => {
    const onGroupByChange = vi.fn();
    const r = renderWithProviders(view({ onGroupByChange }));
    await r.user.click(r.getByRole('combobox', { name: 'Grupla' }));
    await r.user.click(await r.findByRole('option', { name: 'Mevsimsel' }));
    expect(onGroupByChange).toHaveBeenCalledWith('season');
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(view());
    await expectNoAxeViolations(r.container);
  });
});
