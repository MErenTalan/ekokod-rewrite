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

/** The keys each provider's adapter reads; the server sends them with the list. */
export type ProviderEndpointKeys = { provider: Provider; required: string[]; optional: string[] };

const keysOf = (catalog: ProviderEndpointKeys[], provider: Provider) =>
  catalog.find((k) => k.provider === provider) ?? { provider, required: [], optional: [] };

/** Required keys the definition lacks, so a broken definition shows before a fetch fails on it. */
export function missingKeys(
  catalog: ProviderEndpointKeys[],
  provider: Provider,
  endpoints: Record<string, string>,
): string[] {
  return keysOf(catalog, provider).required.filter((k) => !endpoints[k]?.trim());
}

const PROVIDERS: Provider[] = ['osos', 'gridbox', 'aril', 'pm5340', 'isolar'];

export type DefinitionDraft = {
  id?: string;
  provider: Provider;
  subtype: string;
  endpoints: { key: string; url: string }[];
};

export type IntegrationsTabViewProps = {
  definitions: IntegrationDefinition[];
  endpointKeys?: ProviderEndpointKeys[];
  /** `done` closes the dialog: only a successful save closes it, so a refusal stays readable (R199). */
  onSave: (draft: DefinitionDraft, done: () => void) => void;
  onDelete: (definition: IntegrationDefinition) => void;
  fieldErrors?: Record<string, string>;
  saving?: boolean;
  loading?: boolean;
};

const withRequired = (
  catalog: ProviderEndpointKeys[],
  provider: Provider,
  endpoints: DefinitionDraft['endpoints'],
) => [
  ...endpoints,
  ...keysOf(catalog, provider)
    .required.filter((k) => !endpoints.some((e) => e.key === k))
    .map((key) => ({ key, url: '' })),
];

const draftOf = (
  catalog: ProviderEndpointKeys[],
  definition?: IntegrationDefinition,
): DefinitionDraft => {
  const provider = definition?.provider ?? 'osos';
  const endpoints = Object.entries((definition?.endpoints ?? {}) as Record<string, string>).map(
    ([key, url]) => ({ key, url }),
  );
  return {
    id: definition?.id,
    provider,
    subtype: definition?.subtype ?? '',
    endpoints: withRequired(catalog, provider, endpoints),
  };
};

/** The provider catalogue of 01 §7.15 — admin only, and every URL is https (R176). */
export function IntegrationsTabView({
  definitions,
  endpointKeys = [],
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

  const invalidUrl =
    draft?.endpoints.some((e) => e.url !== '' && !e.url.startsWith('https://')) ?? false;
  const draftKeys = draft ? keysOf(endpointKeys, draft.provider) : null;
  const known = draftKeys ? [...draftKeys.required, ...draftKeys.optional] : [];
  const draftMissing = draft
    ? missingKeys(
        endpointKeys,
        draft.provider,
        Object.fromEntries(draft.endpoints.map((e) => [e.key, e.url])),
      )
    : [];

  const columns = useMemo<ColumnDef<IntegrationDefinition, unknown>[]>(
    () => [
      { accessorKey: 'provider', header: t('provider'), enableHiding: false },
      { accessorKey: 'subtype', header: t('subtype') },
      {
        id: 'endpoints',
        header: t('endpoints'),
        accessorFn: (row) => Object.keys((row.endpoints ?? {}) as Record<string, string>).length,
        cell: ({ row }) =>
          t('endpointCount', {
            count: Object.keys((row.original.endpoints ?? {}) as Record<string, string>).length,
          }),
      },
      {
        id: 'health',
        header: t('health'),
        cell: ({ row }) => {
          const missing = missingKeys(
            endpointKeys,
            row.original.provider,
            (row.original.endpoints ?? {}) as Record<string, string>,
          );
          return missing.length ? (
            <span className="text-danger">{t('missingKeys', { keys: missing.join(', ') })}</span>
          ) : (
            <span className="text-success">{t('complete')}</span>
          );
        },
      },
      {
        id: 'actions',
        header: common('actions'),
        cell: ({ row }) => (
          <span className="flex gap-1">
            <IconButton
              label={t('edit')}
              icon={Pencil}
              size="sm"
              onClick={() => setDraft(draftOf(endpointKeys, row.original))}
            />
            <IconButton
              label={t('delete')}
              icon={Trash2}
              size="sm"
              onClick={() => setPendingDelete(row.original)}
            />
          </span>
        ),
      },
    ],
    [t, common, endpointKeys],
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
          <Button iconStart={Plus} onClick={() => setDraft(draftOf(endpointKeys))}>
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
            disabled={invalidUrl || draftMissing.length > 0}
            onClick={() => {
              if (draft) onSave(draft, () => setDraft(null));
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
              onValueChange={(provider) =>
                setDraft({
                  ...draft,
                  provider: provider as Provider,
                  endpoints: withRequired(
                    endpointKeys,
                    provider as Provider,
                    draft.endpoints.filter((e) => e.url),
                  ),
                })
              }
              disabled={Boolean(draft.id)}
            />
            <Input
              label={t('subtype')}
              value={draft.subtype}
              error={fieldErrors.subtype}
              onChange={(e) => setDraft({ ...draft, subtype: e.target.value })}
              required
            />
            {draft.endpoints.map((endpoint, index) => (
              <div key={index} className="flex flex-wrap items-end gap-2">
                <div className="w-56">
                  <Select
                    label={t('endpointKey')}
                    options={[
                      ...known,
                      ...(known.includes(endpoint.key) ? [] : [endpoint.key]),
                    ].map((value) => ({
                      value,
                      label: draftKeys?.required.includes(value) ? `${value} *` : value,
                    }))}
                    value={endpoint.key}
                    error={
                      known.length && !known.includes(endpoint.key)
                        ? t('unknownKey')
                        : fieldErrors[`endpoints.${endpoint.key}`]
                    }
                    onValueChange={(key) =>
                      setDraft({
                        ...draft,
                        endpoints: draft.endpoints.map((e, i) => (i === index ? { ...e, key } : e)),
                      })
                    }
                  />
                </div>
                <div className="min-w-64 flex-1">
                  <Input
                    label={t('endpointUrl')}
                    value={endpoint.url}
                    error={
                      endpoint.url !== '' && !endpoint.url.startsWith('https://')
                        ? t('httpsOnly')
                        : undefined
                    }
                    onChange={(e) =>
                      setDraft({
                        ...draft,
                        endpoints: draft.endpoints.map((x, i) =>
                          i === index ? { ...x, url: e.target.value } : x,
                        ),
                      })
                    }
                  />
                </div>
                <IconButton
                  label={t('removeEndpoint', { key: endpoint.key })}
                  icon={Trash2}
                  size="sm"
                  onClick={() =>
                    setDraft({ ...draft, endpoints: draft.endpoints.filter((_, i) => i !== index) })
                  }
                />
              </div>
            ))}
            {draftMissing.length ? (
              <p role="alert" className="text-sm text-danger">
                {t('missingKeys', { keys: draftMissing.join(', ') })}
              </p>
            ) : null}
            <Button
              variant="secondary"
              size="sm"
              className="self-start"
              disabled={known.every((k) => draft.endpoints.some((e) => e.key === k))}
              onClick={() => {
                const next = known.find((k) => !draft.endpoints.some((e) => e.key === k)) ?? '';
                setDraft({ ...draft, endpoints: [...draft.endpoints, { key: next, url: '' }] });
              }}
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
        {pendingDelete ? (
          <p>
            {t('deleteConfirm', {
              provider: pendingDelete.provider,
              subtype: pendingDelete.subtype,
            })}
          </p>
        ) : null}
      </Dialog>
    </div>
  );
}
