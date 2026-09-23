'use client';

import { useTranslations } from 'next-intl';
import { useEffect, useMemo, useState } from 'react';

import { JobStatusBanner } from '@/components/domain/job-status-banner';
import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import type { Plant } from '@/lib/api/types';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { useJob } from '../jobs/use-job';
import { IsolarLinkView } from './isolar-link-dialog';
import { PlantFormView, emptyPlant, monthlyTargetsComplete, plantDraft, toPlantRequest, type PlantDraft } from './plant-form';
import { PlantsTabView } from './plants-tab';

/** The Solar Plants tab's data (01 §7.15), the iSolar link modal (R281) and the netting analyzer (R289). */
export function PlantsPanel() {
  const t = useTranslations('settings.plants');
  const settings = useTranslations('settings');
  const common = useTranslations('common');
  const { can } = useSession();
  const scope = useScopeParams();
  const [draft, setDraft] = useState<PlantDraft | null>(null);
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
  const analyzers = $api.useQuery('get', '/api/v1/analyzers', { params: { query: { ...scope, limit: 500 } } }, { enabled: canEdit });
  const credentials = $api.useQuery('get', '/api/v1/integration-credentials', { params: { query: scope } }, { enabled: canLink });
  const isolarCredentials = useMemo(
    () => (credentials.data?.items ?? []).filter((c) => c.provider === 'isolar').map((c) => ({ value: c.id, label: `iSolarCloud (${c.subtype})` })),
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
  const link = useApiMutation('post', '/api/v1/plants/{id}/isolar-link', { success: t('linked'), invalidate: ['/api/v1/power-plants'] });
  const unlink = useApiMutation('delete', '/api/v1/plants/{id}/isolar-link', { success: t('unlinked'), invalidate: ['/api/v1/power-plants'] });
  const { job } = useJob(jobId, t('link.job'));

  const value = detail.data && detail.data.id === draft?.id ? plantDraft(detail.data) : draft;
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
        onOpenChange={(open) => !open && setDraft(null)}
        title={draft?.id ? t('edit') : t('add')}
        size="lg"
        footer={
          <Button
            loading={create.isPending || update.isPending}
            disabled={!value || !monthlyTargetsComplete(value)}
            onClick={() => {
              if (!value) return;
              const body = toPlantRequest(value);
              if (value.id) update.mutate({ params: { path: { id: value.id }, query: scope }, body });
              else create.mutate({ params: { query: scope }, body });
              setDraft(null);
            }}
          >
            {common('save')}
          </Button>
        }
      >
        {value ? (
          <PlantFormView
            key={detail.data?.id ?? draft?.id ?? 'new'}
            value={value}
            onChange={setDraft}
            devices={detail.data?.devices ?? []}
            analyzers={analyzerOptions}
            fieldErrors={{ ...create.fieldErrors, ...update.fieldErrors }}
          />
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
