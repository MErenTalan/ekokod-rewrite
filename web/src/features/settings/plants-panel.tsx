'use client';

import { useTranslations } from 'next-intl';
import { useEffect, useMemo, useState } from 'react';

import { JobStatusBanner } from '@/components/domain/job-status-banner';
import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import type { Plant } from '@/lib/api/types';
import { istanbulToday } from '@/lib/dates';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { useJob } from '../jobs/use-job';
import { IsolarLinkView } from './isolar-link-dialog';
import { PlantFormView, emptyPlant, monthlyTargetsComplete, plantDraft, toPlantRequest, toTariffRequest, withTariff, type PlantDraft } from './plant-form';
import { PlantsTabView } from './plants-tab';

/** The Solar Plants tab's data (01 §7.15), the iSolar link modal (R281) and the netting analyzer (R289). */
export function PlantsPanel() {
  const t = useTranslations('settings.plants');
  const settings = useTranslations('settings');
  const common = useTranslations('common');
  const { can } = useSession();
  const scope = useScopeParams();
  const [draft, setDraft] = useState<PlantDraft | null>(null);
  // The plant id whose server copy (with its price) already seeded the draft.
  const [hydrated, setHydrated] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Plant | null>(null);
  const [linking, setLinking] = useState<Plant | null>(null);
  const [pendingUnlink, setPendingUnlink] = useState<Plant | null>(null);
  const [credentialId, setCredentialId] = useState<string | null>(null);
  const [jobId, setJobId] = useState<string | null>(null);
  const canEdit = can('write') && can('settings.plants');
  const canLink = canEdit && can('plants.manage');

  const plants = $api.useQuery('get', '/api/v1/power-plants', { params: { query: { ...scope, limit: 500 } } });
  const detail = $api.useQuery(
    'get',
    '/api/v1/power-plants/{id}',
    { params: { path: { id: draft?.id ?? '' }, query: scope } },
    { enabled: Boolean(draft?.id) },
  );
  const tariffs = $api.useQuery(
    'get',
    '/api/v1/solar-tariffs',
    { params: { query: { ...scope, plant_id: draft?.id ?? '', limit: 1 } } },
    { enabled: Boolean(draft?.id) },
  );
  useEffect(() => {
    if (!draft?.id || hydrated === draft.id || detail.data?.id !== draft.id || tariffs.isPending) return;
    setDraft(withTariff(plantDraft(detail.data), tariffs.data?.items[0]));
    setHydrated(draft.id);
  }, [draft?.id, hydrated, detail.data, tariffs.isPending, tariffs.data]);
  const analyzers = $api.useQuery('get', '/api/v1/analyzers', { params: { query: { ...scope, limit: 500 } } }, { enabled: canEdit });
  const credentials = $api.useQuery('get', '/api/v1/integration-credentials', { params: { query: scope } }, { enabled: canLink });
  const isolarCredentials = useMemo(
    () => (credentials.data?.items ?? []).filter((c) => c.provider === 'isolar' && c.is_active).map((c) => ({ value: c.id, label: `iSolarCloud (${c.subtype})` })),
    [credentials.data],
  );
  useEffect(() => {
    if (!credentialId && isolarCredentials.length > 0) setCredentialId(isolarCredentials[0].value);
  }, [isolarCredentials, credentialId]);
  const account = $api.useQuery('get', '/api/v1/integrations/isolar/plants',
    { params: { query: { ...scope, credential_id: credentialId ?? '' } } }, { enabled: linking !== null && Boolean(credentialId) });

  const create = useApiMutation('post', '/api/v1/power-plants', { success: t('saved'), invalidate: ['/api/v1/power-plants'] });
  const update = useApiMutation('patch', '/api/v1/power-plants/{id}', { success: t('saved'), invalidate: ['/api/v1/power-plants'] });
  const remove = useApiMutation('delete', '/api/v1/power-plants/{id}', { success: t('deleted'), invalidate: ['/api/v1/power-plants'] });
  const priceInvalidate = ['/api/v1/solar-tariffs', '/api/v1/plants', '/api/v1/financial'];
  const saveTariff = useApiMutation('post', '/api/v1/solar-tariffs', { success: t('finance.saved'), invalidate: priceInvalidate });
  const dropTariff = useApiMutation('delete', '/api/v1/solar-tariffs/{id}', { invalidate: priceInvalidate });
  const link = useApiMutation('post', '/api/v1/plants/{id}/isolar-link', { success: t('linked'), invalidate: ['/api/v1/power-plants'] });
  const unlink = useApiMutation('delete', '/api/v1/plants/{id}/isolar-link', { success: t('unlinked'), invalidate: ['/api/v1/power-plants'] });
  const { job } = useJob(jobId, t('link.job'));

  // An edit shows the form only once seeded, so typing is never overwritten by the server copy.
  const value = draft?.id && hydrated !== draft.id ? null : draft;
  const closeDialog = () => {
    setDraft(null);
    setHydrated(null);
  };

  // The plant is saved first; its price follows as a solar tariff (per kWh).
  const savePrice = (saved: PlantDraft, plantId: string) => {
    const body = toTariffRequest(saved, plantId, istanbulToday());
    if (!body) return closeDialog();
    saveTariff.mutate({ params: { query: scope }, body }, {
      onSuccess: () => {
        const old = saved.savedTariff;
        // Same start date means a correction: the old price must not stay beside the new one.
        if (old && old.effective_from === body.effective_from) dropTariff.mutate({ params: { path: { id: old.id }, query: scope } });
        closeDialog();
      },
    });
  };
  const analyzerOptions = (analyzers.data?.items ?? []).map((a) => ({ value: a.id, label: `${a.customer_name ?? a.installation_number} · ${a.installation_number}` }));

  return (
    <>
      <JobStatusBanner job={job} onDismiss={() => setJobId(null)} />
      <PlantsTabView
        plants={plants.data?.items ?? []}
        canEdit={canEdit}
        loading={plants.isLoading}
        onAdd={() => setDraft(emptyPlant())}
        onEdit={(plant) => setDraft({ ...emptyPlant(), id: plant.id, name: plant.name })}
        onDelete={setPendingDelete}
        onLink={canLink ? setLinking : undefined}
        onUnlink={canLink ? setPendingUnlink : undefined}
      />

      <Dialog
        open={draft !== null}
        onOpenChange={(open) => {
          if (open) return;
          closeDialog();
          saveTariff.clearFieldErrors();
          create.clearFieldErrors();
          update.clearFieldErrors();
        }}
        title={draft?.id ? t('edit') : t('add')}
        size="lg"
        footer={
          <Button
            loading={create.isPending || update.isPending || saveTariff.isPending}
            disabled={!value || !monthlyTargetsComplete(value)}
            onClick={() => {
              if (!value) return;
              const body = toPlantRequest(value);
              // The dialog closes only on success, so a refused save can be corrected.
              if (value.id) {
                const id = value.id;
                update.mutate({ params: { path: { id }, query: scope }, body }, { onSuccess: () => savePrice(value, id) });
              } else {
                create.mutate({ params: { query: scope }, body }, {
                  onSuccess: (plant) => {
                    // A refused price then re-saves this plant instead of creating another.
                    setHydrated(plant.id);
                    setDraft({ ...value, id: plant.id });
                    savePrice(value, plant.id);
                  },
                });
              }
            }}
          >
            {common('save')}
          </Button>
        }
      >
        {value ? (
          <PlantFormView
            key={value.id ?? 'new'}
            value={value}
            onChange={setDraft}
            devices={detail.data?.devices ?? []}
            analyzers={analyzerOptions}
            fieldErrors={{ ...create.fieldErrors, ...update.fieldErrors, ...saveTariff.fieldErrors }}
          />
        ) : draft ? (
          <Skeleton className="h-64 w-full" />
        ) : null}
      </Dialog>

      <Dialog open={linking !== null} onOpenChange={(open) => !open && setLinking(null)} title={t('link.title')} size="lg">
        {linking ? (
          <IsolarLinkView
            plantId={linking.id}
            credentials={isolarCredentials}
            credentialId={credentialId}
            onCredentialChange={setCredentialId}
            plants={account.data?.items ?? []}
            loading={account.isPending && Boolean(credentialId)}
            linking={link.isPending}
            onLink={(psId) =>
              link.mutate(
                { params: { path: { id: linking.id }, query: scope }, body: { credential_id: credentialId ?? '', ps_id: psId } },
                { onSuccess: (data) => { setJobId(data.job_id); setLinking(null); } },
              )
            }
          />
        ) : null}
      </Dialog>

      <Dialog
        open={pendingUnlink !== null}
        onOpenChange={(open) => !open && setPendingUnlink(null)}
        title={t('unlinkIsolar')}
        footer={
          <Button
            variant="danger"
            onClick={() => {
              if (pendingUnlink) unlink.mutate({ params: { path: { id: pendingUnlink.id }, query: scope } });
              setPendingUnlink(null);
            }}
          >
            {t('unlinkIsolar')}
          </Button>
        }
      >
        {pendingUnlink ? <p>{t('unlinkConfirm', { name: pendingUnlink.name })}</p> : null}
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
