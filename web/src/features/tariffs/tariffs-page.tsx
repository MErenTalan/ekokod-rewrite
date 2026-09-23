'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { PageHeader } from '@/components/shell/page-header';
import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { EmptyState } from '@/components/ui/empty-state';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { Tabs, type TabItem } from '@/components/ui/tabs';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import { useScopeParams, useSelection } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';
import type { IcmalImport, TariffTemplate } from '@/lib/api/types';

import { BulkTabView } from './bulk-tab';
import { DefaultsTabView, type NationalTariffDraft } from './defaults-tab';
import { IcmalTabView, type IcmalConfirmation } from './icmal-tab';
import { SolarTabView, type SolarTariffDraft } from './solar-tab';
import { draftFrom, emptyDraft, toTariffRequest, type TariffDraft } from './tariff-draft';
import { TariffFormView } from './tariff-form';
import { TariffHistoryView } from './tariff-history';
import { TemplatesTabView, type TemplateApply } from './templates-tab';

const LIST_LIMIT = 200;

/** The tariff screen of 01 §7.11 and its six tabs; a tab whose permission the
 *  principal does not hold is not rendered at all (R247). */
export function TariffsPage() {
  const t = useTranslations('tariffs');
  const common = useTranslations('common');
  const { can } = useSession();
  const scope = useScopeParams();
  const { buildingId } = useSelection();

  const [tab, setTab] = useState('building');
  const [draft, setDraft] = useState<TariffDraft | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [editing, setEditing] = useState<string | null>(null);
  const [templateDraft, setTemplateDraft] = useState<{ id?: string; name: string; description: string; isDefault: boolean; tariff: TariffDraft } | null>(null);
  const [icmalResult, setIcmalResult] = useState<IcmalImport | null>(null);
  const [icmalFailure, setIcmalFailure] = useState<'unreadable' | undefined>();
  const [plantID, setPlantID] = useState<string | null>(null);
  // The bulk tab has no building form behind it, so an assignment carries its
  // own definition: the operator picks buildings, then defines the tariff.
  const [bulkDraft, setBulkDraft] = useState<{ buildingIDs: string[]; tariff: TariffDraft } | null>(null);

  const canEdit = can('tariffs.edit');
  const canTemplates = can('tariffs.templates.read');
  const canBulk = can('tariffs.bulk.read');
  const canIcmal = can('tariffs.icmal');
  const canSolar = can('solar_tariffs.read');
  const canDefaults = can('tariffs.defaults');

  const invalidate = ['/api/v1/tariffs'];
  const tariffs = $api.useQuery(
    'get',
    '/api/v1/tariffs',
    { params: { query: { ...scope, limit: LIST_LIMIT, ...(buildingId ? { building_id: buildingId } : {}) } } },
    { enabled: Boolean(buildingId) && tab === 'building' },
  );
  const version = $api.useQuery(
    'get',
    '/api/v1/tariffs/{id}',
    { params: { path: { id: editing ?? '' }, query: scope } },
    { enabled: editing !== null },
  );
  if (editing && version.data && draft?.id !== editing) setDraft(draftFrom(version.data));

  const templates = $api.useQuery(
    'get',
    '/api/v1/tariff-templates',
    { params: { query: { ...scope, limit: LIST_LIMIT } } },
    { enabled: canTemplates && (tab === 'templates' || tab === 'bulk') },
  );
  const buildingStates = $api.useQuery(
    'get',
    '/api/v1/buildings/bulk-tariff/current',
    { params: { query: { ...scope, limit: LIST_LIMIT } } },
    { enabled: canBulk && (tab === 'templates' || tab === 'bulk') },
  );
  const assignments = $api.useQuery(
    'get',
    '/api/v1/buildings/bulk-tariff/history',
    { params: { query: { ...scope, limit: LIST_LIMIT } } },
    { enabled: canBulk && tab === 'bulk' },
  );
  const plants = $api.useQuery(
    'get',
    '/api/v1/power-plants',
    { params: { query: { ...scope, limit: LIST_LIMIT } } },
    { enabled: canSolar && tab === 'solar' },
  );
  const solarTariffs = $api.useQuery(
    'get',
    '/api/v1/solar-tariffs',
    { params: { query: { ...scope, plant_id: plantID ?? '', limit: LIST_LIMIT } } },
    { enabled: canSolar && tab === 'solar' && plantID !== null },
  );
  const defaults = $api.useQuery(
    'get',
    '/api/v1/national-tariff-schedule',
    { params: { query: { limit: LIST_LIMIT } } },
    { enabled: canDefaults && tab === 'defaults' },
  );

  const create = useApiMutation('post', '/api/v1/tariffs', { success: t('saved'), invalidate });
  const update = useApiMutation('patch', '/api/v1/tariffs/{id}', { success: t('saved'), invalidate });
  const remove = useApiMutation('delete', '/api/v1/tariffs/{id}', { success: t('deleted'), invalidate });
  const createTemplate = useApiMutation('post', '/api/v1/tariff-templates', { success: t('templates.saved'), invalidate });
  const updateTemplate = useApiMutation('patch', '/api/v1/tariff-templates/{id}', { success: t('templates.saved'), invalidate });
  const deleteTemplate = useApiMutation('delete', '/api/v1/tariff-templates/{id}', { success: t('templates.deleted'), invalidate });
  const applyTemplate = useApiMutation('post', '/api/v1/tariff-templates/{id}/apply', { success: t('templates.saved'), invalidate });
  const bulkAssign = useApiMutation('post', '/api/v1/buildings/bulk-tariff', { success: t('saved'), invalidate });
  const uploadIcmal = useApiMutation('post', '/api/v1/icmal-imports', { invalidate: [] });
  const applyIcmal = useApiMutation('post', '/api/v1/icmal-imports/{id}/apply', { success: t('saved'), invalidate });
  const createSolar = useApiMutation('post', '/api/v1/solar-tariffs', { success: t('solar.saved'), invalidate });
  const deleteSolar = useApiMutation('delete', '/api/v1/solar-tariffs/{id}', { success: t('solar.deleted'), invalidate });
  const publishDefault = useApiMutation('post', '/api/v1/national-tariff-schedule', { success: t('defaults.saved'), invalidate });
  const deleteDefault = useApiMutation('delete', '/api/v1/national-tariff-schedule/{id}', { success: t('defaults.deleted'), invalidate });

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

  const submitTemplate = () => {
    if (!templateDraft || templateDraft.name.trim() === '') return;
    const body = {
      name: templateDraft.name,
      description: templateDraft.description || undefined,
      is_default: templateDraft.isDefault,
      tariff: toTariffRequest({ ...templateDraft.tariff, buildingId: '' }),
    };
    const done = () => setTemplateDraft(null);
    if (templateDraft.id) {
      updateTemplate.mutate({ params: { path: { id: templateDraft.id }, query: scope }, body }, { onSuccess: done });
    } else {
      createTemplate.mutate({ params: { query: scope }, body }, { onSuccess: done });
    }
  };

  const submitBulk = () => {
    if (!bulkDraft) return;
    bulkAssign.mutate(
      {
        params: { query: scope },
        body: { building_ids: bulkDraft.buildingIDs, tariff: toTariffRequest({ ...bulkDraft.tariff, buildingId: '' }) },
      },
      { onSuccess: () => setBulkDraft(null) },
    );
  };

  const apply = ({ templateID, buildingIDs, effectiveFrom }: TemplateApply) =>
    applyTemplate.mutate({
      params: { path: { id: templateID }, query: scope },
      body: { building_ids: buildingIDs, effective_from: effectiveFrom },
    });

  const uploadFile = (file: File) => {
    setIcmalFailure(undefined);
    const body = new FormData();
    body.append('file', file);
    uploadIcmal.mutate(
      { params: { query: scope }, body: body as never, bodySerializer: (b: unknown) => b } as never,
      {
        onSuccess: (data) => setIcmalResult(data as IcmalImport),
        onError: () => setIcmalFailure('unreadable'),
      },
    );
  };

  const confirmIcmal = (confirmations: IcmalConfirmation[]) => {
    if (!icmalResult) return;
    applyIcmal.mutate({ params: { path: { id: icmalResult.id }, query: scope }, body: { confirmations } });
  };

  const templateFromStored = (template: TariffTemplate) =>
    setTemplateDraft({
      id: template.id,
      name: template.name,
      description: template.description ?? '',
      isDefault: template.is_default ?? false,
      tariff: draftFrom({ ...template.tariff, id: '', created_at: '', updated_at: '' } as never),
    });

  const items: TabItem[] = [];
  items.push({
    value: 'building',
    label: t('tabs.building'),
    content: !buildingId ? (
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
    ),
  });
  if (canTemplates) {
    items.push({
      value: 'templates',
      label: t('tabs.templates'),
      content: (
        <TemplatesTabView
          templates={templates.data?.items ?? []}
          buildings={buildingStates.data?.items ?? []}
          canEdit={canEdit}
          loading={templates.isPending}
          onCreate={() => setTemplateDraft({ name: '', description: '', isDefault: false, tariff: emptyDraft() })}
          onEdit={templateFromStored}
          onDelete={(template) =>
            deleteTemplate.mutate({ params: { path: { id: template.id }, query: scope } })
          }
          onApply={apply}
        />
      ),
    });
  }
  if (canBulk) {
    items.push({
      value: 'bulk',
      label: t('tabs.bulk'),
      content: (
        <BulkTabView
          buildings={buildingStates.data?.items ?? []}
          assignments={assignments.data?.items ?? []}
          canEdit={canEdit}
          loading={buildingStates.isPending}
          onAssign={(buildingIDs) => setBulkDraft({ buildingIDs, tariff: emptyDraft() })}
        />
      ),
    });
  }
  if (canIcmal) {
    items.push({
      value: 'icmal',
      label: t('tabs.icmal'),
      content: (
        <IcmalTabView
          result={icmalResult}
          buildings={buildingStates.data?.items ?? []}
          onUpload={uploadFile}
          onApply={confirmIcmal}
          failure={icmalFailure}
          uploading={uploadIcmal.isPending}
          applying={applyIcmal.isPending}
        />
      ),
    });
  }
  if (canSolar) {
    items.push({
      value: 'solar',
      label: t('tabs.solar'),
      content: (
        <SolarTabView
          plants={(plants.data?.items ?? []).map((p) => ({ id: p.id, name: p.name }))}
          plantID={plantID}
          onPlantChange={setPlantID}
          tariffs={solarTariffs.data?.items ?? []}
          canEdit={canEdit}
          loading={solarTariffs.isPending && plantID !== null}
          onCreate={(body: SolarTariffDraft) => createSolar.mutate({ params: { query: scope }, body })}
          onDelete={(id) => deleteSolar.mutate({ params: { path: { id }, query: scope } })}
        />
      ),
    });
  }
  if (canDefaults) {
    items.push({
      value: 'defaults',
      label: t('tabs.defaults'),
      content: (
        <DefaultsTabView
          entries={defaults.data?.items ?? []}
          loading={defaults.isPending}
          onPublish={(body: NationalTariffDraft) => publishDefault.mutate({ params: { query: {} }, body })}
          onDelete={(id) => deleteDefault.mutate({ params: { path: { id }, query: {} } })}
        />
      ),
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />

      <Tabs items={items} value={tab} onValueChange={setTab} />

      <Dialog
        open={templateDraft !== null}
        onOpenChange={(next) => !next && setTemplateDraft(null)}
        title={templateDraft?.id ? t('templates.edit') : t('templates.new')}
        size="lg"
        footer={
          <>
            <Button variant="ghost" onClick={() => setTemplateDraft(null)}>{common('cancel')}</Button>
            <Button loading={createTemplate.isPending || updateTemplate.isPending} onClick={submitTemplate}>
              {common('save')}
            </Button>
          </>
        }
      >
        {templateDraft ? (
          <div className="flex flex-col gap-4">
            <Input
              label={t('templates.name')}
              required
              value={templateDraft.name}
              onChange={(e) => setTemplateDraft({ ...templateDraft, name: e.target.value })}
            />
            <Input
              label={t('templates.descriptionField')}
              value={templateDraft.description}
              onChange={(e) => setTemplateDraft({ ...templateDraft, description: e.target.value })}
            />
            <Switch
              label={t('templates.isDefault')}
              checked={templateDraft.isDefault}
              onCheckedChange={(isDefault) => setTemplateDraft({ ...templateDraft, isDefault })}
            />
            {/* One form, two callers: the template dialog edits the same
                definition a building tariff does (R249). */}
            <TariffFormView
              draft={templateDraft.tariff}
              onDraftChange={(tariff) => setTemplateDraft({ ...templateDraft, tariff })}
              onSubmit={submitTemplate}
              serverErrors={templateDraft.id ? updateTemplate.fieldErrors : createTemplate.fieldErrors}
            />
          </div>
        ) : null}
      </Dialog>

      <Dialog
        open={bulkDraft !== null}
        onOpenChange={(next) => !next && setBulkDraft(null)}
        title={t('bulk.title')}
        size="lg"
        footer={
          <>
            <Button variant="ghost" onClick={() => setBulkDraft(null)}>{common('cancel')}</Button>
            <Button loading={bulkAssign.isPending} onClick={submitBulk}>{t('bulk.apply')}</Button>
          </>
        }
      >
        {bulkDraft ? (
          <div className="flex flex-col gap-4">
            <p className="text-foreground-muted type-caption">{t('bulk.selected', { count: bulkDraft.buildingIDs.length })}</p>
            <TariffFormView
              draft={bulkDraft.tariff}
              onDraftChange={(tariff) => setBulkDraft({ ...bulkDraft, tariff })}
              onSubmit={submitBulk}
              serverErrors={bulkAssign.fieldErrors}
              saving={bulkAssign.isPending}
            />
          </div>
        ) : null}
      </Dialog>

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
