'use client';

import type { ColumnDef } from '@tanstack/react-table';
import { Plus } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useMemo, useState } from 'react';

import type { JobView } from '@/components/domain/job-status-banner';
import { JobStatusBanner } from '@/components/domain/job-status-banner';
import { Button } from '@/components/ui/button';
import { DataTable } from '@/components/ui/data-table';
import { DatePicker } from '@/components/ui/date-picker';
import { Dialog } from '@/components/ui/dialog';
import { StatusBadge } from '@/components/ui/status-badge';
import type { Building, IntegrationCredential, IntegrationDefinition } from '@/lib/api/types';
import { formatDate } from '@/lib/format';
import type { Locale } from '@/i18n/locale';
import { useLocale } from 'next-intl';

import { CredentialFormView, emptyDraft, type CredentialDraft } from './credential-form';

export type BackfillRange = { from: string; to: string };

export type CredentialsSectionViewProps = {
  credentials: IntegrationCredential[];
  definitions: IntegrationDefinition[];
  buildings: Building[];
  /** Creating a credential needs the definition catalogue, which only admins read (R201). */
  canCreate: boolean;
  onSave: (draft: CredentialDraft) => void;
  onVerify: (credential: IntegrationCredential) => void;
  onDiscover: (credential: IntegrationCredential) => void;
  onBackfill: (credential: IntegrationCredential, range: BackfillRange) => void;
  onDelete: (credential: IntegrationCredential) => void;
  onConnectIsolar: (credential: IntegrationCredential) => void;
  job?: JobView | null;
  onDismissJob?: () => void;
  fieldErrors?: Record<string, string>;
  saving?: boolean;
  loading?: boolean;
  today: string;
};

/** The configured integrations of 01 §7.15; secrets are never shown (R177). */
export function CredentialsSectionView({
  credentials,
  definitions,
  buildings,
  canCreate,
  onSave,
  onVerify,
  onDiscover,
  onBackfill,
  onDelete,
  onConnectIsolar,
  job = null,
  onDismissJob,
  fieldErrors = {},
  saving = false,
  loading = false,
  today,
}: CredentialsSectionViewProps) {
  const t = useTranslations('settings.credentials');
  const settings = useTranslations('settings');
  const common = useTranslations('common');
  const locale = useLocale() as Locale;
  const [draft, setDraft] = useState<CredentialDraft | null>(null);
  const [pendingDelete, setPendingDelete] = useState<IntegrationCredential | null>(null);
  const [backfill, setBackfill] = useState<{ credential: IntegrationCredential; range: BackfillRange } | null>(null);

  const columns = useMemo<ColumnDef<IntegrationCredential, unknown>[]>(
    () => [
      {
        id: 'provider',
        header: t('provider'),
        accessorFn: (row) => `${row.provider} · ${row.subtype}`,
        enableHiding: false,
      },
      { id: 'username', header: t('username'), accessorFn: (row) => row.username ?? '—' },
      {
        id: 'secret',
        header: t('status'),
        accessorFn: (row) => (row.has_secret ? 1 : 0),
        cell: ({ row }) =>
          row.original.has_secret ? <StatusBadge status="success" label={t('secretStored')} /> : <StatusBadge status="neutral" label="—" />,
      },
      {
        id: 'verified',
        header: t('lastVerified'),
        accessorFn: (row) => row.last_verified_at ?? '',
        cell: ({ row }) =>
          row.original.last_verified_at ? formatDate(row.original.last_verified_at.slice(0, 10), locale) : t('never'),
      },
    ],
    [t, locale],
  );

  return (
    <div className="flex flex-col gap-4">
      <JobStatusBanner job={job} onDismiss={onDismissJob} />
      <DataTable
        columns={columns}
        data={credentials}
        caption={t('title')}
        getRowId={(row) => row.id}
        loading={loading}
        empty={{ title: t('empty'), description: t('emptyHint') }}
        rowActions={(row) => [
          { type: 'item' as const, label: t('verify'), onSelect: () => onVerify(row) },
          { type: 'item' as const, label: t('discover'), onSelect: () => onDiscover(row) },
          {
            type: 'item' as const,
            label: t('backfill'),
            onSelect: () => setBackfill({ credential: row, range: { from: today, to: today } }),
          },
          ...(row.provider === 'isolar'
            ? [{ type: 'item' as const, label: t('connectIsolar'), onSelect: () => onConnectIsolar(row) }]
            : []),
          { type: 'item' as const, label: t('delete'), onSelect: () => setPendingDelete(row) },
        ]}
        toolbar={
          canCreate ? (
            <Button iconStart={Plus} onClick={() => setDraft(emptyDraft())}>
              {t('add')}
            </Button>
          ) : undefined
        }
      />

      <Dialog
        open={draft !== null}
        onOpenChange={(open) => !open && setDraft(null)}
        title={t('add')}
        footer={
          <Button
            loading={saving}
            onClick={() => {
              if (draft) onSave(draft);
              setDraft(null);
            }}
          >
            {common('save')}
          </Button>
        }
      >
        {draft ? (
          <CredentialFormView
            definitions={definitions}
            buildings={buildings}
            value={draft}
            onChange={setDraft}
            mode="create"
            fieldErrors={fieldErrors}
          />
        ) : null}
      </Dialog>

      <Dialog
        open={backfill !== null}
        onOpenChange={(open) => !open && setBackfill(null)}
        title={t('backfillTitle')}
        footer={
          <Button
            onClick={() => {
              if (backfill) onBackfill(backfill.credential, backfill.range);
              setBackfill(null);
            }}
          >
            {t('backfill')}
          </Button>
        }
      >
        {backfill ? (
          <div className="flex flex-col gap-4">
            <DatePicker
              label={t('backfillFrom')}
              value={backfill.range.from}
              max={today}
              onValueChange={(from) => setBackfill({ ...backfill, range: { ...backfill.range, from: from ?? today } })}
            />
            <DatePicker
              label={t('backfillTo')}
              value={backfill.range.to}
              max={today}
              onValueChange={(to) => setBackfill({ ...backfill, range: { ...backfill.range, to: to ?? today } })}
            />
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
        {pendingDelete ? <p>{t('deleteConfirm', { provider: pendingDelete.provider, subtype: pendingDelete.subtype })}</p> : null}
      </Dialog>
    </div>
  );
}
