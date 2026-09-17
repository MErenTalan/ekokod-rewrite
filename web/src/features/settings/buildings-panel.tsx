'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import type { Building } from '@/lib/api/types';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { BuildingFormView, buildingDraft, emptyBuilding, toBuildingRequest, type BuildingDraft } from './building-form';
import { BuildingsTabView } from './buildings-tab';

/** The Buildings tab's data (01 §7.15); the tariff editor itself is F8's. */
export function BuildingsPanel() {
  const t = useTranslations('settings.buildings');
  const settings = useTranslations('settings');
  const common = useTranslations('common');
  const { can } = useSession();
  const scope = useScopeParams();
  const [draft, setDraft] = useState<BuildingDraft | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Building | null>(null);
  const canEdit = can('write') && can('settings.buildings');

  const buildings = $api.useQuery('get', '/api/v1/buildings', {
    params: { query: { ...scope, include: ['analyzer_count', 'active_status'], limit: 500 } },
  });
  const users = $api.useQuery('get', '/api/v1/users', { params: { query: { ...scope, limit: 500, is_active: true } } }, { enabled: canEdit });
  const detail = $api.useQuery(
    'get',
    '/api/v1/buildings/{id}',
    { params: { path: { id: draft?.id ?? '' }, query: scope } },
    { enabled: Boolean(draft?.id) },
  );

  const create = useApiMutation('post', '/api/v1/buildings', { success: t('saved'), invalidate: ['/api/v1/buildings'] });
  const update = useApiMutation('patch', '/api/v1/buildings/{id}', { success: t('saved'), invalidate: ['/api/v1/buildings'] });
  const remove = useApiMutation('delete', '/api/v1/buildings/{id}', { success: t('deleted'), invalidate: ['/api/v1/buildings'] });

  return (
    <>
      <BuildingsTabView
        buildings={buildings.data?.items ?? []}
        canEdit={canEdit}
        loading={buildings.isLoading}
        onAdd={() => setDraft(emptyBuilding())}
        onEdit={(building) => setDraft({ ...emptyBuilding(), id: building.id, name: building.name })}
        onDelete={setPendingDelete}
      />

      <Dialog
        open={draft !== null}
        onOpenChange={(open) => !open && setDraft(null)}
        title={draft?.id ? t('edit') : t('add')}
        size="lg"
        footer={
          <Button
            loading={create.isPending || update.isPending}
            onClick={() => {
              if (!draft) return;
              const body = toBuildingRequest(draft);
              if (draft.id) update.mutate({ params: { path: { id: draft.id }, query: scope }, body });
              else create.mutate({ params: { query: scope }, body });
              setDraft(null);
            }}
          >
            {common('save')}
          </Button>
        }
      >
        {draft ? (
          <BuildingFormView
            // The detail arrives after the dialog opens, so the form remounts with it.
            key={detail.data?.id ?? draft.id ?? 'new'}
            value={detail.data && detail.data.id === draft.id ? buildingDraft(detail.data) : draft}
            onChange={setDraft}
            users={users.data?.items ?? []}
            tariffHistory={detail.data?.tariff_history ?? []}
            fieldErrors={{ ...create.fieldErrors, ...update.fieldErrors }}
          />
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
              if (pendingDelete) remove.mutate({ params: { path: { id: pendingDelete.id }, query: scope } });
              setPendingDelete(null);
            }}
          >
            {settings('deleteAction')}
          </Button>
        }
      >
        {pendingDelete ? <p>{t('deleteConfirm', { name: pendingDelete.name })}</p> : null}
      </Dialog>
    </>
  );
}
