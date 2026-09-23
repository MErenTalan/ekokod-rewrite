import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { activity } from './_fixture';
import { StatusTable, type StatusTableProps } from './status-table';

const rows = [
  activity(),
  activity({ id: 'a-2', sub_category: 'sub_grid_electricity', scope: 'scope_2', iso_category: 'category_2', status: 'approved', is_automated: true,
    quantity: '100', unit: 'kWh', emission_kgco2e: '46.9', period_start: '2026-09-09', period_end: '2026-09-09' }),
  activity({ id: 'a-3', status: 'rejected' }),
];

const render = (over: Partial<StatusTableProps> = {}) => {
  const props: StatusTableProps = {
    rows, editable: true, filters: { status: 'all', scope: 'all' }, onFilters: vi.fn(), onApprove: vi.fn(), onReject: vi.fn(),
    onEdit: vi.fn(), onDelete: vi.fn(), hasMore: false, onLoadMore: vi.fn(), ...over,
  };
  return { props, r: renderWithProviders(<StatusTable {...props} />) };
};

describe('StatusTable', () => {
  it('shows each record with its status as icon and text (R325)', () => {
    const { r } = render();
    expect(r.getByText('Onay Bekliyor')).toBeVisible();
    expect(r.getByText('Onaylandı')).toBeVisible();
    expect(r.getByText('Reddedildi')).toBeVisible();
    expect(r.getAllByText('2.066,72')).toHaveLength(2);
    expect(r.getAllByText('1.000 m3')).toHaveLength(2);
    expect(r.getByText('Otomatik')).toBeVisible();
  });

  it('approves, rejects and edits from the row', async () => {
    const { r, props } = render();
    const first = r.getAllByRole('row')[1];
    await r.user.click(r.getAllByRole('button', { name: 'Onayla' })[0]);
    expect(props.onApprove).toHaveBeenCalledWith(rows[0]);
    await r.user.click(r.getAllByRole('button', { name: 'Reddet' })[0]);
    expect(props.onReject).toHaveBeenCalledWith(rows[0]);
    await r.user.click(r.getAllByRole('button', { name: 'Düzenle' })[0]);
    expect(props.onEdit).toHaveBeenCalledWith(rows[0]);
    expect(first).toBeVisible();
  });

  it('never offers edit or delete on an automated record (R306)', () => {
    const { r } = render({ rows: [rows[1]] });
    expect(r.getByRole('button', { name: 'Reddet' })).toBeVisible();
    expect(r.queryByRole('button', { name: 'Düzenle' })).toBeNull();
    expect(r.queryByRole('button', { name: 'Sil' })).toBeNull();
  });

  it('shows a read-only role no actions', () => {
    const { r } = render({ editable: false });
    expect(r.queryByRole('button', { name: 'Onayla' })).toBeNull();
    expect(r.queryByRole('columnheader', { name: 'İşlemler' })).toBeNull();
  });

  it('says so when nothing matches', () => {
    const { r } = render({ rows: [] });
    expect(r.getByText('Kayıt yok')).toBeVisible();
  });
});
