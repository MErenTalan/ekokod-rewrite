'use client';

import type { ColumnDef } from '@tanstack/react-table';
import { Plus } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useMemo, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { DataTable } from '@/components/ui/data-table';
import { Dialog } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { StatusBadge } from '@/components/ui/status-badge';
import type { Company, CompanyCreateRequest } from '@/lib/api/types';

export type CompanyListViewProps = {
  companies: Company[];
  ownCompanyId: string;
  /** The company the admin is currently acting for (R139/R209). */
  activeCompanyId?: string;
  hasMore: boolean;
  onLoadMore: () => void;
  onActAs: (companyId: string | undefined) => void;
  onCreate: (values: CompanyCreateRequest) => void;
  onDelete: (company: Company) => void;
  saving?: boolean;
  loading?: boolean;
};

/** Every tenant, for the admin (05 §3), with the company switcher's own action. */
export function CompanyListView({
  companies,
  ownCompanyId,
  activeCompanyId,
  hasMore,
  onLoadMore,
  onActAs,
  onCreate,
  onDelete,
  saving = false,
  loading = false,
}: CompanyListViewProps) {
  const t = useTranslations('settings.company');
  const settings = useTranslations('settings');
  const common = useTranslations('common');
  const [draft, setDraft] = useState<CompanyCreateRequest | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Company | null>(null);

  const columns = useMemo<ColumnDef<Company, unknown>[]>(
    () => [
      { accessorKey: 'name', header: t('name'), enableHiding: false },
      { id: 'sector', header: t('sector'), accessorFn: (row) => row.sector ?? '—' },
      {
        id: 'active',
        // Not t('actAs'): the sortable column header would collide with the row button.
        header: t('scopeColumn'),
        accessorFn: (row) => (row.id === activeCompanyId ? 1 : 0),
        cell: ({ row }) =>
          row.original.id === activeCompanyId ? (
            <StatusBadge status="info" label={t('actingAs')} />
          ) : (
            <Button size="sm" variant="ghost" onClick={() => onActAs(row.original.id === ownCompanyId ? undefined : row.original.id)}>
              {t('actAs')}
            </Button>
          ),
      },
      {
        id: 'delete',
        header: t('delete'),
        cell: ({ row }) => (
          <Button
            size="sm"
            variant="ghost"
            disabled={row.original.id === ownCompanyId}
            title={row.original.id === ownCompanyId ? t('cannotDeleteOwn') : undefined}
            onClick={() => setPendingDelete(row.original)}
          >
            {settings('deleteAction')}
          </Button>
        ),
      },
    ],
    [t, settings, activeCompanyId, ownCompanyId, onActAs],
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('list')}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <DataTable
          columns={columns}
          data={companies}
          caption={t('list')}
          getRowId={(row) => row.id}
          loading={loading}
          empty={{ title: t('empty'), description: t('emptyHint') }}
          toolbar={
            <Button iconStart={Plus} onClick={() => setDraft({ name: '' })}>
              {t('add')}
            </Button>
          }
        />
        {hasMore ? (
          <Button variant="secondary" size="sm" className="self-start" onClick={onLoadMore}>
            {t('loadMore')}
          </Button>
        ) : null}
      </CardContent>

      <Dialog
        open={draft !== null}
        onOpenChange={(open) => !open && setDraft(null)}
        title={t('add')}
        footer={
          <Button
            loading={saving}
            onClick={() => {
              if (draft) onCreate(draft);
              setDraft(null);
            }}
          >
            {common('save')}
          </Button>
        }
      >
        {draft ? (
          <div className="flex flex-col gap-4">
            <Input label={t('name')} value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} required />
            <Input label={t('sector')} value={draft.sector ?? ''} onChange={(e) => setDraft({ ...draft, sector: e.target.value })} />
          </div>
        ) : null}
      </Dialog>

      <Dialog
        open={pendingDelete !== null}
        onOpenChange={(open) => !open && setPendingDelete(null)}
        title={settings('confirmDelete')}
        footer={
          <Button
            variant="danger"
            onClick={() => {
              if (pendingDelete) onDelete(pendingDelete);
              setPendingDelete(null);
            }}
          >
            {settings('deleteAction')}
          </Button>
        }
      >
        {pendingDelete ? <p>{t('deleteConfirm', { name: pendingDelete.name })}</p> : null}
      </Dialog>
    </Card>
  );
}
