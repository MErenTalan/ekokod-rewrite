'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { PageHeader } from '@/components/shell/page-header';
import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { EmptyState } from '@/components/ui/empty-state';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import { useSelection, useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { draftFrom, emptyDraft, toTariffRequest, type TariffDraft } from './tariff-draft';
import { TariffFormView } from './tariff-form';
import { TariffHistoryView } from './tariff-history';

const LIST_LIMIT = 200;

/** The tariff screen of 01 §7.11. F8a ships the building-tariff tab; the
 *  template, bulk, icmal, solar and default tabs join it as their endpoints
 *  land. */
export function TariffsPage() {
  const t = useTranslations('tariffs');
  const common = useTranslations('common');
  const { can } = useSession();
  const scope = useScopeParams();
  const { buildingId } = useSelection();

  const [draft, setDraft] = useState<TariffDraft | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [editing, setEditing] = useState<string | null>(null);

  const canEdit = can('tariffs.edit');

  const query = { ...scope, limit: LIST_LIMIT, ...(buildingId ? { building_id: buildingId } : {}) };
  const tariffs = $api.useQuery('get', '/api/v1/tariffs', { params: { query } }, { enabled: Boolean(buildingId) });

  // The list carries only a summary, so the form loads the version it edits.
  const version = $api.useQuery(
    'get',
    '/api/v1/tariffs/{id}',
    { params: { path: { id: editing ?? '' }, query: scope } },
    { enabled: editing !== null },
  );
  if (editing && version.data && draft?.id !== editing) setDraft(draftFrom(version.data));

  const invalidate = ['/api/v1/tariffs'];
  const create = useApiMutation('post', '/api/v1/tariffs', { success: t('saved'), invalidate });
  const update = useApiMutation('patch', '/api/v1/tariffs/{id}', { success: t('saved'), invalidate });
  const remove = useApiMutation('delete', '/api/v1/tariffs/{id}', { success: t('deleted'), invalidate });

  const close = () => {
    setDraft(null);
    setEditing(null);
  };

  const submit = () => {
    if (!draft) return;
    const body = toTariffRequest({ ...draft, buildingId: draft.buildingId || (buildingId ?? '') });
    if (draft.id) {
      update.mutate({ params: { path: { id: draft.id }, query: scope }, body }, { onSuccess: close });
    } else {
      create.mutate({ params: { query: scope }, body }, { onSuccess: close });
    }
  };

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />

      {!buildingId ? (
        <EmptyState title={t('selectBuilding')} description={t('buildingRequired')} />
      ) : draft ? (
        <TariffFormView
          draft={draft}
          onDraftChange={setDraft}
          onSubmit={submit}
          onCancel={close}
          serverErrors={draft.id ? update.fieldErrors : create.fieldErrors}
          saving={create.isPending || update.isPending}
        />
      ) : (
        <TariffHistoryView
          tariffs={tariffs.data?.items ?? []}
          canEdit={canEdit}
          loading={tariffs.isPending}
          onCreate={() => setDraft({ ...emptyDraft(), buildingId })}
          onEdit={setEditing}
          onDelete={setDeleting}
        />
      )}

      <Dialog
        open={deleting !== null}
        onOpenChange={(next) => !next && setDeleting(null)}
        title={t('deleteTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setDeleting(null)}>{common('cancel')}</Button>
            <Button
              variant="danger"
              loading={remove.isPending}
              onClick={() =>
                deleting &&
                remove.mutate({ params: { path: { id: deleting }, query: scope } }, { onSuccess: () => setDeleting(null) })
              }
            >
              {common('delete')}
            </Button>
          </>
        }
      >
        <p className="text-foreground type-body">{t('deleteBody')}</p>
      </Dialog>
    </div>
  );
}
