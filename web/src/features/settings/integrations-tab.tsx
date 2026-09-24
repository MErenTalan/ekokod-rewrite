'use client';

import type { ColumnDef } from '@tanstack/react-table';
import { Pencil, Plus, Trash2 } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useMemo, useState } from 'react';

import { Button } from '@/components/ui/button';
import { DataTable } from '@/components/ui/data-table';
import { Dialog } from '@/components/ui/dialog';
import { IconButton } from '@/components/ui/icon-button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import type { IntegrationDefinition, Provider } from '@/lib/api/types';

/** The endpoint templates a provider may declare (R176). */
export const ENDPOINT_KEYS = [
  'authentication',
  'analyzer_list',
  'hourly_values',
  'energy_values',
  'token',
  'last_index',
  'indexes',
  'last_success_date',
  'load_profiles',
  'owner_consumptions',
  'current_indexes',
  'end_of_month_indexes',
] as const;

const PROVIDERS: Provider[] = ['osos', 'gridbox', 'aril', 'pm5340', 'isolar'];

export type DefinitionDraft = {
  id?: string;
  provider: Provider;
  subtype: string;
  endpoints: { key: string; url: string }[];
};

export type IntegrationsTabViewProps = {
  definitions: IntegrationDefinition[];
  onSave: (draft: DefinitionDraft) => void;
  onDelete: (definition: IntegrationDefinition) => void;
  fieldErrors?: Record<string, string>;
  saving?: boolean;
  loading?: boolean;
};

const draftOf = (definition?: IntegrationDefinition): DefinitionDraft => ({
  id: definition?.id,
  provider: definition?.provider ?? 'osos',
  subtype: definition?.subtype ?? '',
  endpoints: Object.entries((definition?.endpoints ?? {}) as Record<string, string>).map(([key, url]) => ({ key, url })),
});

/** The provider catalogue of 01 §7.15 — admin only, and every URL is https (R176). */
export function IntegrationsTabView({
  definitions,
  onSave,
  onDelete,
  fieldErrors = {},
  saving = false,
  loading = false,
}: IntegrationsTabViewProps) {
  const t = useTranslations('settings.integrations');
  const settings = useTranslations('settings');
  const common = useTranslations('common');
  const [draft, setDraft] = useState<DefinitionDraft | null>(null);
  const [pendingDelete, setPendingDelete] = useState<IntegrationDefinition | null>(null);

  const invalidUrl = draft?.endpoints.some((e) => e.url !== '' && !e.url.startsWith('https://')) ?? false;

  const columns = useMemo<ColumnDef<IntegrationDefinition, unknown>[]>(
    () => [
      { accessorKey: 'provider', header: t('provider'), enableHiding: false },
      { accessorKey: 'subtype', header: t('subtype') },
      {
        id: 'endpoints',
        header: t('endpoints'),
        accessorFn: (row) => Object.keys((row.endpoints ?? {}) as Record<string, string>).length,
        cell: ({ row }) => t('endpointCount', { count: Object.keys((row.original.endpoints ?? {}) as Record<string, string>).length }),
      },
      {
        id: 'actions',
        header: common('actions'),
        cell: ({ row }) => (
          <span className="flex gap-1">
            <IconButton label={t('edit')} icon={Pencil} size="sm" onClick={() => setDraft(draftOf(row.original))} />
            <IconButton label={t('delete')} icon={Trash2} size="sm" onClick={() => setPendingDelete(row.original)} />
          </span>
        ),
      },
    ],
    [t, common],
  );

  return (
    <div className="flex flex-col gap-4">
      <DataTable
        columns={columns}
        data={definitions}
        caption={t('title')}
        getRowId={(row) => row.id}
        loading={loading}
        empty={{ title: t('empty'), description: t('emptyHint') }}
        toolbar={
          <Button iconStart={Plus} onClick={() => setDraft(draftOf())}>
            {t('add')}
          </Button>
        }
      />

      <Dialog
        open={draft !== null}
        onOpenChange={(open) => !open && setDraft(null)}
        title={draft?.id ? t('edit') : t('add')}
        size="lg"
        footer={
          <Button
            loading={saving}
            disabled={invalidUrl}
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
          <div className="flex flex-col gap-4">
            <Select
              label={t('provider')}
              options={PROVIDERS.map((value) => ({ value, label: value }))}
              value={draft.provider}
              onValueChange={(provider) => setDraft({ ...draft, provider: provider as Provider })}
              disabled={Boolean(draft.id)}
            />
            <Input label={t('subtype')} value={draft.subtype} error={fieldErrors.subtype} onChange={(e) => setDraft({ ...draft, subtype: e.target.value })} required />
            {draft.endpoints.map((endpoint, index) => (
              <div key={index} className="flex flex-wrap items-end gap-2">
                <div className="w-56">
                  <Select
                    label={t('endpointKey')}
                    options={ENDPOINT_KEYS.map((value) => ({ value, label: value }))}
                    value={endpoint.key}
                    onValueChange={(key) =>
                      setDraft({ ...draft, endpoints: draft.endpoints.map((e, i) => (i === index ? { ...e, key } : e)) })
                    }
                  />
                </div>
                <div className="min-w-64 flex-1">
                  <Input
                    label={t('endpointUrl')}
                    value={endpoint.url}
                    error={endpoint.url !== '' && !endpoint.url.startsWith('https://') ? t('httpsOnly') : undefined}
                    onChange={(e) =>
                      setDraft({ ...draft, endpoints: draft.endpoints.map((x, i) => (i === index ? { ...x, url: e.target.value } : x)) })
                    }
                  />
                </div>
                <IconButton
                  label={t('removeEndpoint', { key: endpoint.key })}
                  icon={Trash2}
                  size="sm"
                  onClick={() => setDraft({ ...draft, endpoints: draft.endpoints.filter((_, i) => i !== index) })}
                />
              </div>
            ))}
            <Button
              variant="secondary"
              size="sm"
              className="self-start"
              onClick={() => setDraft({ ...draft, endpoints: [...draft.endpoints, { key: ENDPOINT_KEYS[0], url: '' }] })}
            >
              {t('addEndpoint')}
            </Button>
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
