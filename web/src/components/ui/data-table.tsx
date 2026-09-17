'use client';

import {
  flexRender,
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  type Column,
  type ColumnDef,
  type RowData,
  type SortingState,
  type Table as TanTable,
  type VisibilityState,
} from '@tanstack/react-table';
import { ArrowDown, ArrowUp, ArrowUpDown, Columns3, MoreHorizontal } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useState, type MutableRefObject, type ReactNode } from 'react';

import { cn } from '@/lib/cn';

import { Button } from './button';
import { Checkbox } from './checkbox';
import { DropdownMenu, type DropdownMenuItem } from './dropdown-menu';
import { EmptyState } from './empty-state';
import { IconButton } from './icon-button';
import { Pagination } from './pagination';
import { Popover } from './popover';
import { Skeleton } from './skeleton';
import { Table, TableBody, TableCaption, TableCell, TableContainer, TableHead, TableHeader, TableRow } from './table';

declare module '@tanstack/react-table' {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  interface ColumnMeta<TData extends RowData, TValue> {
    numeric?: boolean;
    /** Text for CSV/Excel export; defaults to the raw cell value. */
    exportValue?: (row: TData) => string;
  }
}

export type DataTableProps<T> = {
  columns: ColumnDef<T, unknown>[];
  data: T[];
  caption: string;
  getRowId: (row: T) => string;
  initialSorting?: SortingState;
  pageSize?: number | false;
  enableColumnVisibility?: boolean;
  rowActions?: (row: T) => DropdownMenuItem[];
  /** Names each row's action button ("Actions for Merkez Bina"). */
  getRowLabel?: (row: T) => string;
  toolbar?: ReactNode;
  loading?: boolean;
  empty: { title: string; description: string; action?: ReactNode };
  maxHeight?: string;
  /** Exposes the TanStack instance, e.g. for getExportRows in an ExportMenu. */
  tableRef?: MutableRefObject<TanTable<T> | null>;
};

const headerText = <T,>(column: Column<T, unknown>) => {
  const header = column.columnDef.header;
  return typeof header === 'string' ? header : column.id;
};

/** Visible columns only, sorted rows across all pages, formatted text (plan D22). */
export function getExportRows<T>(table: TanTable<T>): string[][] {
  const columns = table.getVisibleLeafColumns().filter((c) => c.accessorFn);
  return [
    columns.map(headerText),
    ...table.getSortedRowModel().rows.map((row) =>
      columns.map((c) => c.columnDef.meta?.exportValue?.(row.original) ?? String(row.getValue(c.id) ?? '')),
    ),
  ];
}

const sortIcons = { asc: ArrowUp, desc: ArrowDown, false: ArrowUpDown } as const;
const SKELETON_ROWS = 5;

export function DataTable<T>({
  columns,
  data,
  caption,
  getRowId,
  initialSorting = [],
  pageSize = 25,
  enableColumnVisibility = false,
  rowActions,
  getRowLabel,
  toolbar,
  loading = false,
  empty,
  maxHeight,
  tableRef,
}: DataTableProps<T>) {
  const t = useTranslations('table');
  const [sorting, setSorting] = useState<SortingState>(initialSorting);
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({});
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: pageSize || data.length || 1 });
  const table = useReactTable({
    data,
    columns,
    getRowId: (row) => getRowId(row),
    state: { sorting, columnVisibility, pagination },
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    onPaginationChange: setPagination,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    ...(pageSize === false ? {} : { getPaginationRowModel: getPaginationRowModel() }),
  });
  if (tableRef) tableRef.current = table;
  const columnCount = table.getVisibleLeafColumns().length + (rowActions ? 1 : 0);
  const rows = table.getRowModel().rows;

  return (
    <div className="flex min-w-0 flex-col gap-3">
      {toolbar || enableColumnVisibility ? (
        <div className="flex flex-wrap items-center gap-2">
          {toolbar}
          {enableColumnVisibility ? (
            <div className="ms-auto">
              <Popover label={t('columns')} align="end" trigger={<Button variant="secondary" size="sm" iconStart={Columns3}>{t('columns')}</Button>}>
                <div className="flex min-w-44 flex-col gap-1">
                  {table
                    .getAllLeafColumns()
                    .filter((c) => c.getCanHide())
                    .map((c) => (
                      <Checkbox key={c.id} label={headerText(c)} checked={c.getIsVisible()} onCheckedChange={(v) => c.toggleVisibility(v)} />
                    ))}
                </div>
              </Popover>
            </div>
          ) : null}
        </div>
      ) : null}
      <TableContainer label={caption} style={maxHeight ? { maxHeight } : undefined}>
        <Table>
          <TableCaption>{caption}</TableCaption>
          <TableHeader className={cn(maxHeight && 'sticky top-0 z-10')}>
            {table.getHeaderGroups().map((group) => (
              <TableRow key={group.id}>
                {group.headers.map((header) => {
                  const numeric = header.column.columnDef.meta?.numeric;
                  const sorted = header.column.getIsSorted();
                  const SortIcon = sortIcons[sorted === false ? 'false' : sorted];
                  const label = header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext());
                  return (
                    <TableHead
                      key={header.id}
                      numeric={numeric}
                      aria-sort={header.column.getCanSort() ? (sorted === 'asc' ? 'ascending' : sorted === 'desc' ? 'descending' : 'none') : undefined}
                    >
                      {header.column.getCanSort() ? (
                        <button
                          type="button"
                          onClick={header.column.getToggleSortingHandler()}
                          className={cn('inline-flex items-center gap-1 rounded-sm hover:text-foreground pointer-coarse:min-h-11', numeric && 'flex-row-reverse')}
                        >
                          {label}
                          <SortIcon aria-hidden className="size-3.5" />
                        </button>
                      ) : (
                        label
                      )}
                    </TableHead>
                  );
                })}
                {rowActions ? (
                  <TableHead className="w-12">
                    <span className="sr-only">{t('actions')}</span>
                  </TableHead>
                ) : null}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: SKELETON_ROWS }, (_, i) => (
                <TableRow key={i} data-skeleton-row>
                  {Array.from({ length: columnCount }, (_, j) => (
                    <TableCell key={j}>
                      <Skeleton className="h-5 w-full" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={columnCount}>
                  <EmptyState title={empty.title} description={empty.description} action={empty.action} />
                </TableCell>
              </TableRow>
            ) : (
              rows.map((row) => (
                <TableRow key={row.id} className="hover:bg-surface-sunken">
                  {row.getVisibleCells().map((c) => (
                    <TableCell key={c.id} numeric={c.column.columnDef.meta?.numeric}>
                      {flexRender(c.column.columnDef.cell, c.getContext())}
                    </TableCell>
                  ))}
                  {rowActions ? (
                    <TableCell className="py-1">
                      <DropdownMenu
                        trigger={
                          <IconButton
                            label={getRowLabel ? t('rowActions', { label: getRowLabel(row.original) }) : t('actions')}
                            icon={MoreHorizontal}
                            size="sm"
                            tooltip={false}
                          />
                        }
                        items={rowActions(row.original)}
                      />
                    </TableCell>
                  ) : null}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </TableContainer>
      {pageSize !== false && table.getPageCount() > 1 ? (
        <Pagination
          page={pagination.pageIndex + 1}
          pageCount={table.getPageCount()}
          onPageChange={(p) => table.setPageIndex(p - 1)}
          pageSize={pagination.pageSize}
          pageSizeOptions={[25, 50, 100]}
          onPageSizeChange={(size) => table.setPageSize(size)}
          totalItems={data.length}
        />
      ) : null}
    </div>
  );
}
