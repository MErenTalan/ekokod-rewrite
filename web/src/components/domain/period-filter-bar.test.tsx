import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { PeriodFilterBar, periodPresets } from './period-filter-bar';

const base = {
  granularity: 'daily' as const,
  onGranularityChange: () => {},
  range: { from: '2026-09-01', to: '2026-09-17' },
  onApply: () => {},
  today: '2026-09-17',
};

describe('PeriodFilterBar', () => {
  it('presets are computed from the injected today', async () => {
    const onRangeChange = vi.fn();
    const { getByRole, findByRole, user } = renderWithProviders(<PeriodFilterBar {...base} onRangeChange={onRangeChange} />);
    const expectations: [string, { from: string; to: string }][] = [
      ['Son 7 gün', { from: '2026-09-11', to: '2026-09-17' }],
      ['Geçen yıl', { from: '2025-01-01', to: '2025-12-31' }],
      ['Geçen ay', { from: '2026-08-01', to: '2026-08-31' }],
      ['Son 6 ay', { from: '2026-04-01', to: '2026-09-17' }],
      ['Bu yıl', { from: '2026-01-01', to: '2026-09-17' }],
    ];
    for (const [label, range] of expectations) {
      await user.click(getByRole('button', { name: /Dönem/ }));
      await user.click(await findByRole('button', { name: label }));
      expect(onRangeChange).toHaveBeenLastCalledWith(range);
    }
  });

  it('presets handle month and year boundaries', () => {
    const march = Object.fromEntries(periodPresets('2026-03-31').map((p) => [p.id, p.range]));
    expect(march.lastMonth).toEqual({ from: '2026-02-01', to: '2026-02-28' });
    expect(march.last6Months).toEqual({ from: '2025-10-01', to: '2026-03-31' });
    const january = Object.fromEntries(periodPresets('2024-01-03').map((p) => [p.id, p.range]));
    expect(january.last7Days).toEqual({ from: '2023-12-28', to: '2024-01-03' });
    expect(january.lastMonth).toEqual({ from: '2023-12-01', to: '2023-12-31' });
  });

  it('apply is a button that reports busy', () => {
    const { getByRole } = renderWithProviders(<PeriodFilterBar {...base} onRangeChange={() => {}} applying />);
    expect(getByRole('button', { name: /Uygula/ })).toHaveAttribute('aria-busy', 'true');
  });

  it('only allowed granularities are offered', async () => {
    const { getByRole, findAllByRole, user } = renderWithProviders(
      <PeriodFilterBar {...base} onRangeChange={() => {}} allowedGranularities={['monthly', 'yearly']} granularity="monthly" />,
    );
    await user.click(getByRole('combobox', { name: 'Çözünürlük' }));
    expect((await findAllByRole('option')).map((o) => o.textContent)).toEqual(['Aylık', 'Yıllık']);
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<PeriodFilterBar {...base} onRangeChange={() => {}} />);
    await expectNoAxeViolations(container);
  });
});
