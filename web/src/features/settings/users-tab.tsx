'use client';

import type { ColumnDef } from '@tanstack/react-table';
import { Pencil, Plus, Trash2 } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useMemo } from 'react';

import { Button } from '@/components/ui/button';
import { DataTable } from '@/components/ui/data-table';
import { IconButton } from '@/components/ui/icon-button';
import { StatusBadge } from '@/components/ui/status-badge';
import type { User } from '@/lib/api/types';
import { formatDate } from '@/lib/format';
import type { Locale } from '@/i18n/locale';
import { useLocale } from 'next-intl';

import { ROLE_LABEL_KEY } from './user-form';

export type UsersTabViewProps = {
  users: User[];
  /** The signed-in user: their own row cannot be deleted here (R174). */
  selfId: string;
  canEdit: boolean;
  onAdd: () => void;
  onEdit: (user: User) => void;
  onDelete: (user: User) => void;
  loading?: boolean;
};

/** The user list of 01 §7.15. */
export function UsersTabView({ users, selfId, canEdit, onAdd, onEdit, onDelete, loading = false }: UsersTabViewProps) {
  const t = useTranslations('settings.users');
  const roles = useTranslations('shell.roles');
  const common = useTranslations('common');
  const locale = useLocale() as Locale;

  const columns = useMemo<ColumnDef<User, unknown>[]>(() => {
    const base: ColumnDef<User, unknown>[] = [
      { accessorKey: 'name', header: t('name'), enableHiding: false },
      { accessorKey: 'email', header: t('email') },
      { id: 'phone', header: t('phone'), accessorFn: (row) => row.phone ?? '—' },
      { id: 'role', header: t('role'), accessorFn: (row) => row.role, cell: ({ row }) => roles(ROLE_LABEL_KEY[row.original.role]) },
      {
        id: 'active',
        header: t('active'),
        accessorFn: (row) => (row.is_active ? 1 : 0),
        cell: ({ row }) => (
          <StatusBadge status={row.original.is_active ? 'success' : 'neutral'} label={row.original.is_active ? t('active') : '—'} />
        ),
      },
      {
        id: 'lastLogin',
        header: t('lastLogin'),
        accessorFn: (row) => row.last_login_at ?? '',
        cell: ({ row }) => (row.original.last_login_at ? formatDate(row.original.last_login_at.slice(0, 10), locale) : t('never')),
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
            {/* The caller's own row keeps its delete visible but disabled (R174). */}
            <IconButton
              label={t('delete')}
              icon={Trash2}
              size="sm"
              disabled={row.original.id === selfId}
              onClick={() => onDelete(row.original)}
            />
          </span>
        ),
      },
    ];
  }, [t, roles, common, locale, canEdit, onEdit, onDelete, selfId]);

  return (
    <DataTable
      columns={columns}
      data={users}
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
