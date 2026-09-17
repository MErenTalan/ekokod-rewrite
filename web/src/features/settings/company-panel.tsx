'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { api } from '@/lib/api/client';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import { istanbulToday } from '@/lib/dates';
import { useScopeParams, useSelection } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';
import { useJob } from '@/features/jobs/use-job';

import { CompanyListView } from './company-list';
import { CompanyTabView } from './company-tab';
import { CredentialsSectionView } from './credentials-section';
import type { CredentialDraft } from './credential-form';
import { toCredentialRequest } from './credential-form';

/** The Company tab's data: details, the admin company list and the integrations. */
export function CompanyPanel() {
  const t = useTranslations('settings.company');
  const credentials = useTranslations('settings.credentials');
  const { me, can } = useSession();
  const scope = useScopeParams();
  const { companyId, set } = useSelection();
  const [cursor, setCursor] = useState<string | undefined>();
  const [startedJob, setStartedJob] = useState<{ id: string; label: string } | null>(null);
  const today = istanbulToday();

  const companyIdInScope = scope.company_id ?? me.company.id;
  const company = $api.useQuery('get', '/api/v1/companies/{id}', {
    params: { path: { id: companyIdInScope }, query: scope },
  });
  const saveCompany = useApiMutation('patch', '/api/v1/companies/{id}', {
    success: t('saved'),
    invalidate: ['/api/v1/companies'],
  });

  const companies = $api.useQuery(
    'get',
    '/api/v1/companies',
    { params: { query: { limit: 50, ...(cursor ? { cursor } : {}) } } },
    { enabled: can('admin.companies') },
  );
  const createCompany = useApiMutation('post', '/api/v1/companies', { success: t('created'), invalidate: ['/api/v1/companies'] });
  const deleteCompany = useApiMutation('delete', '/api/v1/companies/{id}', { success: t('deleted'), invalidate: ['/api/v1/companies'] });

  const definitions = $api.useQuery('get', '/api/v1/integration-definitions', {}, { enabled: can('settings.integrations') });
  const buildings = $api.useQuery('get', '/api/v1/buildings', { params: { query: { ...scope, limit: 500 } } });
  const credentialList = $api.useQuery(
    'get',
    '/api/v1/integration-credentials',
    { params: { query: scope } },
    { enabled: can('integrations.credentials') },
  );
  const saveCredential = useApiMutation('post', '/api/v1/integration-credentials', {
    success: credentials('saved'),
    invalidate: ['/api/v1/integration-credentials'],
  });
  const verify = useApiMutation('post', '/api/v1/integration-credentials/{id}/verify', { success: credentials('verified') });
  const discover = useApiMutation('post', '/api/v1/integration-credentials/{id}/discover', {
    success: credentials('discoverStarted'),
  });
  const backfill = useApiMutation('post', '/api/v1/integration-credentials/{id}/backfill', {
    success: credentials('backfillStarted'),
  });
  const removeCredential = useApiMutation('delete', '/api/v1/integration-credentials/{id}', {
    success: credentials('deleted'),
    invalidate: ['/api/v1/integration-credentials'],
  });
  const { job } = useJob(startedJob?.id ?? null, startedJob?.label ?? '');

  const section = can('integrations.credentials') ? (
    <CredentialsSectionView
      credentials={credentialList.data?.items ?? []}
      definitions={definitions.data?.items ?? []}
      buildings={buildings.data?.items ?? []}
      canCreate={can('settings.integrations')}
      loading={credentialList.isLoading}
      saving={saveCredential.isPending}
      fieldErrors={saveCredential.fieldErrors}
      today={today}
      job={job}
      onDismissJob={() => setStartedJob(null)}
      onSave={(draft: CredentialDraft) => saveCredential.mutate({ params: { query: scope }, body: toCredentialRequest(draft) })}
      onVerify={(credential) => verify.mutate({ params: { path: { id: credential.id }, query: scope } })}
      onDiscover={(credential) =>
        discover.mutate(
          { params: { path: { id: credential.id }, query: scope } },
          { onSuccess: (data) => setStartedJob({ id: (data as { job_id: string }).job_id, label: credentials('discover') }) },
        )
      }
      onBackfill={(credential, range) =>
        backfill.mutate(
          { params: { path: { id: credential.id }, query: scope }, body: { from: `${range.from}T00:00:00+03:00`, to: `${range.to}T00:00:00+03:00` } },
          { onSuccess: (data) => setStartedJob({ id: (data as { job_id: string }).job_id, label: credentials('backfill') }) },
        )
      }
      onDelete={(credential) => removeCredential.mutate({ params: { path: { id: credential.id }, query: scope } })}
      onConnectIsolar={async (credential) => {
        const { data } = await api.GET('/api/v1/integrations/isolar/authorize-url', {
          params: { query: { ...scope, credential_id: credential.id } },
        });
        if (data?.url) window.location.assign(data.url);
      }}
    />
  ) : null;

  return (
    <CompanyTabView
      // The form seeds its fields from the company, so it remounts when it arrives.
      key={company.data?.id ?? 'loading'}
      company={company.data ?? null}
      canEdit={can('settings.company.edit')}
      saving={saveCompany.isPending}
      fieldErrors={saveCompany.fieldErrors}
      onSave={(values) => saveCompany.mutate({ params: { path: { id: companyIdInScope }, query: scope }, body: values })}
    >
      {can('admin.companies') ? (
        <CompanyListView
          companies={companies.data?.items ?? []}
          ownCompanyId={me.company.id}
          activeCompanyId={companyId}
          hasMore={Boolean(companies.data?.next_cursor)}
          loading={companies.isLoading}
          saving={createCompany.isPending}
          onLoadMore={() => setCursor(companies.data?.next_cursor ?? undefined)}
          onActAs={(id) => set({ companyId: id })}
          onCreate={(values) => createCompany.mutate({ body: values })}
          onDelete={(target) => deleteCompany.mutate({ params: { path: { id: target.id } } })}
        />
      ) : null}
      {section}
    </CompanyTabView>
  );
}
