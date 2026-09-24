'use client';

import type { ColumnDef } from '@tanstack/react-table';
import { Pencil, Plus, Trash2 } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useMemo } from 'react';

import { Button } from '@/components/ui/button';
import { DataTable } from '@/components/ui/data-table';
import { IconButton } from '@/components/ui/icon-button';
import { StatusBadge } from '@/components/ui/status-badge';
import type { Building } from '@/lib/api/types';

export type BuildingsTabViewProps = {
  buildings: Building[];
  /** Read-only roles see the list and nothing that changes it (01 §2). */
  canEdit: boolean;
  onAdd: () => void;
  onEdit: (building: Building) => void;
  onDelete: (building: Building) => void;
  loading?: boolean;
};

/** The building list of 01 §7.15. */
export function BuildingsTabView({ buildings, canEdit, onAdd, onEdit, onDelete, loading = false }: BuildingsTabViewProps) {
  const t = useTranslations('settings.buildings');
  const common = useTranslations('common');

  const columns = useMemo<ColumnDef<Building, unknown>[]>(() => {
    const base: ColumnDef<Building, unknown>[] = [
      { accessorKey: 'name', header: t('name'), enableHiding: false },
      { id: 'address', header: t('address'), accessorFn: (row) => row.address ?? '—' },
      { id: 'sector', header: t('sector'), accessorFn: (row) => row.sector ?? '—' },
      { id: 'cutoff', header: t('cutoffDay'), accessorFn: (row) => row.bill_cutoff_day, meta: { numeric: true } },
      { id: 'analyzers', header: t('analyzerCount'), accessorFn: (row) => row.analyzer_count ?? 0, meta: { numeric: true } },
      {
        id: 'status',
        header: t('status'),
        accessorFn: (row) => (row.activity_status === 'active' ? 1 : 0),
        cell: ({ row }) => (
          <StatusBadge
            status={row.original.activity_status === 'active' ? 'success' : 'neutral'}
            label={row.original.activity_status === 'active' ? 'Aktif' : 'Pasif'}
          />
        ),
      },
    ];
    if (!canEdit) return base;
    return [
      ...base,
      {
        id: 'actions',
        header: common('actions'),
        cell: ({ row }) => (
          <span className="flex gap-1">
            <IconButton label={t('edit')} icon={Pencil} size="sm" onClick={() => onEdit(row.original)} />
            <IconButton label={t('delete')} icon={Trash2} size="sm" onClick={() => onDelete(row.original)} />
          </span>
        ),
      },
    ];
  }, [t, common, canEdit, onEdit, onDelete]);

  return (
    <DataTable
      columns={columns}
      data={buildings}
      caption={t('title')}
      getRowId={(row) => row.id}
      loading={loading}
      empty={{ title: t('empty'), description: t('emptyHint') }}
      toolbar={
        canEdit ? (
          <Button iconStart={Plus} onClick={onAdd}>
            {t('add')}
          </Button>
        ) : undefined
      }
    />
  );
}
