import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { factor } from './_fixture';
import { FactorTable, type FactorTableProps } from './factor-table';

const rows = [factor(), factor({ id: 'f-own', key: 'grid_electricity_tr_2022', label: 'Şebeke elektriği', main_category: 'cat_electricity',
  base_factor: '0.44', base_unit: 'kWh', overridden: true, platform_base_factor: '0.469', source: 'Kendi ölçümümüz', source_year: 2026 })];

const render = (over: Partial<FactorTableProps> = {}) => {
  const props: FactorTableProps = { rows, editable: true, query: '', onQuery: vi.fn(), main: 'all', onMain: vi.fn(), onOverride: vi.fn(), onReset: vi.fn(), ...over };
  return { props, r: renderWithProviders(<FactorTable {...props} />) };
};

describe('FactorTable', () => {
  it('shows each factor with unit, source year and the platform value of an override (R326)', () => {
    const { r } = render();
    expect(r.getByText('2,06672 / m3')).toBeVisible();
    expect(r.getByText('Defra, 2025')).toBeVisible();
    expect(r.getByText('Şirkete özel')).toBeVisible();
    expect(r.getByText('Platform: 0,469')).toBeVisible();
  });

  it('opens the override for a row and asks to reset', async () => {
    const { r, props } = render();
    await r.user.click(r.getByRole('button', { name: 'Şebeke elektriği faktörünü değiştir' }));
    expect(props.onOverride).toHaveBeenCalledWith(rows[1]);
    await r.user.click(r.getByRole('button', { name: 'Varsayılana dön' }));
    expect(props.onReset).toHaveBeenCalled();
  });

  it('passes the search on', async () => {
    const { r, props } = render();
    await r.user.type(r.getByRole('searchbox', { name: 'Faktör ara' }), 'g');
    expect(props.onQuery).toHaveBeenCalledWith('g');
  });

  it('shows a read-only role no change controls', () => {
    const { r } = render({ editable: false });
    expect(r.queryByRole('button', { name: /faktörünü değiştir/ })).toBeNull();
    expect(r.queryByRole('button', { name: 'Varsayılana dön' })).toBeNull();
  });

  it('says so when nothing matches', () => {
    const { r } = render({ rows: [] });
    expect(r.getByText('Eşleşen faktör yok')).toBeVisible();
  });
});
