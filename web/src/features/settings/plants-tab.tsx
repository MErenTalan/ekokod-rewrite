'use client';

import type { ColumnDef } from '@tanstack/react-table';
import { Pencil, Plus, Trash2 } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useMemo } from 'react';

import { Button } from '@/components/ui/button';
import { DataTable } from '@/components/ui/data-table';
import { IconButton } from '@/components/ui/icon-button';
import type { Plant } from '@/lib/api/types';
import { formatNumber } from '@/lib/format';

export type PlantsTabViewProps = {
  plants: Plant[];
  canEdit: boolean;
  onAdd: () => void;
  onEdit: (plant: Plant) => void;
  onDelete: (plant: Plant) => void;
  loading?: boolean;
};

/** The solar plant list of 01 §7.15. */
export function PlantsTabView({ plants, canEdit, onAdd, onEdit, onDelete, loading = false }: PlantsTabViewProps) {
  const t = useTranslations('settings.plants');
  const common = useTranslations('common');

  const columns = useMemo<ColumnDef<Plant, unknown>[]>(() => {
    const base: ColumnDef<Plant, unknown>[] = [
      { accessorKey: 'name', header: t('name'), enableHiding: false },
      { id: 'kind', header: t('kind'), accessorFn: (row) => row.plant_kind, cell: ({ row }) => t(row.original.plant_kind) },
      { id: 'installation', header: t('installationNumber'), accessorFn: (row) => row.installation_number ?? '—' },
      {
        id: 'capacity',
        header: t('capacity'),
        accessorFn: (row) => Number(row.total_capacity_kw ?? 0),
        meta: { numeric: true },
        cell: ({ row }) => formatNumber(row.original.total_capacity_kw ?? null),
      },
      { id: 'isolar', header: 'iSolar', accessorFn: (row) => row.isolar_ps_name ?? '—' },
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
      data={plants}
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
