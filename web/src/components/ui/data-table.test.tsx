import type { ColumnDef, Table as TanTable } from '@tanstack/react-table';
import { within } from '@testing-library/react';
import { createRef } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { DataTable, getExportRows } from './data-table';

type Row = { id: string; name: string; kwh: string };
const columns: ColumnDef<Row, unknown>[] = [
  { accessorKey: 'name', header: 'Bina' },
  { accessorKey: 'kwh', header: 'Tüketim', meta: { numeric: true, exportValue: (r: Row) => r.kwh.replace('.', ',') }, sortingFn: (a, b) => Number(a.original.kwh) - Number(b.original.kwh) },
];
const rows = (n: number): Row[] => Array.from({ length: n }, (_, i) => ({ id: `b${i}`, name: `Bina ${i}`, kwh: String((i * 37) % 100) }));
const empty = { title: 'Bina yok', description: 'Bir bina ekleyin.' };

describe('DataTable', () => {
  it('sorts by clicking a header', async () => {
    const { getByRole, user, container } = renderWithProviders(
      <DataTable columns={columns} data={rows(4)} caption="Binalar" getRowId={(r) => r.id} empty={empty} />,
    );
    await user.click(getByRole('button', { name: /Tüketim/ }));
    const th = getByRole('columnheader', { name: /Tüketim/ });
    expect(th).toHaveAttribute('aria-sort', 'ascending');
    const values = [...container.querySelectorAll('tbody tr')].map((tr) => Number(tr.children[1].textContent));
    expect(values).toEqual([...values].sort((a, b) => a - b));
  });

  it('hides a column from the visibility menu', async () => {
    const tableRef = createRef<TanTable<Row> | null>() as React.MutableRefObject<TanTable<Row> | null>;
    const { getByRole, findByRole, queryByRole, user } = renderWithProviders(
      <DataTable columns={columns} data={rows(3)} caption="Binalar" getRowId={(r) => r.id} empty={empty} enableColumnVisibility tableRef={tableRef} />,
    );
    await user.click(getByRole('button', { name: 'Sütunlar' }));
    await user.click(await findByRole('checkbox', { name: 'Tüketim' }));
    expect(queryByRole('columnheader', { name: /Tüketim/ })).not.toBeInTheDocument();
    expect(getExportRows(tableRef.current!)[0]).toEqual(['Bina']);
  });

  it('export rows use visible columns and exportValue', () => {
    const tableRef = { current: null } as React.MutableRefObject<TanTable<Row> | null>;
    renderWithProviders(<DataTable columns={columns} data={[{ id: 'a', name: 'Merkez', kwh: '12.5' }]} caption="Binalar" getRowId={(r) => r.id} empty={empty} tableRef={tableRef} />);
    expect(getExportRows(tableRef.current!)).toEqual([['Bina', 'Tüketim'], ['Merkez', '12,5']]);
  });

  it('paginates at pageSize', () => {
    const { container } = renderWithProviders(<DataTable columns={columns} data={rows(30)} caption="Binalar" getRowId={(r) => r.id} empty={empty} />);
    expect(container.querySelectorAll('tbody tr')).toHaveLength(25);
  });

  it('loading renders skeleton rows, empty renders EmptyState', () => {
    const { container, rerender, getByText } = renderWithProviders(
      <DataTable columns={columns} data={[]} caption="Binalar" getRowId={(r) => r.id} empty={empty} loading />,
    );
    expect(container.querySelectorAll('[data-skeleton-row]')).toHaveLength(5);
    rerender(<DataTable columns={columns} data={[]} caption="Binalar" getRowId={(r) => r.id} empty={empty} />);
    expect(getByText('Bina yok')).toBeInTheDocument();
  });

  it('row actions are a labelled menu', async () => {
    const onEdit = vi.fn();
    const { getByRole, findByRole, user } = renderWithProviders(
      <DataTable
        columns={columns}
        data={rows(1)}
        caption="Binalar"
        getRowId={(r) => r.id}
        getRowLabel={(r) => r.name}
        empty={empty}
        rowActions={(r) => [{ type: 'item', label: 'Düzenle', onSelect: () => onEdit(r.id) }]}
      />,
    );
    await user.click(getByRole('button', { name: /İşlemler/ }));
    const menu = await findByRole('menu');
    await user.click(within(menu).getByRole('menuitem', { name: 'Düzenle' }));
    expect(onEdit).toHaveBeenCalledWith('b0');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(
      <DataTable columns={columns} data={rows(3)} caption="Binalar" getRowId={(r) => r.id} empty={empty} enableColumnVisibility rowActions={() => []} />,
    );
    await expectNoAxeViolations(container);
  });
});
