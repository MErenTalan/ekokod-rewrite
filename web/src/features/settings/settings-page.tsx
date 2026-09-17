'use client';

import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useEffect, useMemo, type ReactNode } from 'react';

import { PageHeader } from '@/components/shell/page-header';
import { Tabs } from '@/components/ui/tabs';
import { useToast } from '@/components/ui/toast';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';
import { useLogout } from '@/lib/session/use-logout';

import { AccountTabView } from './account-tab';
import { BuildingsPanel } from './buildings-panel';
import { CompanyPanel } from './company-panel';
import { PlantsPanel } from './plants-panel';
import { IntegrationsTabView } from './integrations-tab';
import { SmtpTabView } from './smtp-tab';
import { resolveTab, visibleTabs, type SettingsTabId } from './tabs';

/**
 * The tabbed settings screen of 01 §7.15. Which tabs exist at all is decided by
 * the permission table (R200); the API enforces the same matrix.
 */
export function SettingsPage({ slots }: { slots?: Partial<Record<SettingsTabId, ReactNode>> }) {
  const t = useTranslations('settings');
  const account = useTranslations('settings.account');
  const smtpText = useTranslations('settings.smtp');
  const integrationsText = useTranslations('settings.integrations');
  const { me, can } = useSession();
  const scope = useScopeParams();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const toast = useToast();
  const logout = useLogout();

  const visible = useMemo(() => visibleTabs(can), [can]);
  const requested = params.get('tab');
  const active = resolveTab(requested, visible);
  const isDemo = me.role === 'demo';

  useEffect(() => {
    // A forbidden or unknown tab in the URL is corrected, never silently ignored.
    if (requested && requested !== active) {
      const next = new URLSearchParams(params.toString());
      next.set('tab', active);
      router.replace(`${pathname}?${next.toString()}`);
    }
  }, [requested, active, params, pathname, router]);

  useEffect(() => {
    // The iSolar OAuth callback comes back here (R187).
    const isolar = params.get('isolar');
    if (!isolar) return;
    toast.toast({ tone: isolar === 'ok' ? 'success' : 'danger', title: integrationsText(isolar === 'ok' ? 'saved' : 'empty') });
    const next = new URLSearchParams(params.toString());
    next.delete('isolar');
    router.replace(`${pathname}?${next.toString()}`);
  }, [params, pathname, router, toast, integrationsText]);

  const sessions = $api.useQuery('get', '/api/v1/auth/sessions', {}, { enabled: !isDemo });
  const profile = useApiMutation('patch', '/api/v1/profile', { success: account('saved'), invalidate: ['/api/v1/auth/me'] });
  const password = useApiMutation('post', '/api/v1/auth/change-password', { success: account('passwordChanged') });
  const revoke = useApiMutation('delete', '/api/v1/auth/sessions/{id}', {
    success: account('sessionRevoked'),
    invalidate: ['/api/v1/auth/sessions'],
  });
  // Signing out everywhere ends this session too, so it finishes like a logout.
  const logoutAll = useApiMutation('post', '/api/v1/auth/logout-all', { onSuccess: () => void logout() });

  const smtp = $api.useQuery('get', '/api/v1/smtp-settings', { params: { query: scope } }, { enabled: can('settings.smtp'), retry: false });
  const saveSmtp = useApiMutation('put', '/api/v1/smtp-settings', { success: smtpText('saved'), invalidate: ['/api/v1/smtp-settings'] });
  const testSmtp = useApiMutation('post', '/api/v1/smtp-settings/test', { success: smtpText('testSent') });

  const definitions = $api.useQuery('get', '/api/v1/integration-definitions', {}, { enabled: can('settings.integrations') });
  const createDefinition = useApiMutation('post', '/api/v1/integration-definitions', {
    success: integrationsText('saved'),
    invalidate: ['/api/v1/integration-definitions'],
  });
  const updateDefinition = useApiMutation('patch', '/api/v1/integration-definitions/{id}', {
    success: integrationsText('saved'),
    invalidate: ['/api/v1/integration-definitions'],
  });
  const deleteDefinition = useApiMutation('delete', '/api/v1/integration-definitions/{id}', {
    success: integrationsText('deleted'),
    invalidate: ['/api/v1/integration-definitions'],
  });

  const content: Record<SettingsTabId, ReactNode> = {
    account: (
      <AccountTabView
        profile={me}
        readOnly={isDemo}
        sessions={sessions.data?.items ?? null}
        fieldErrors={{ ...profile.fieldErrors, ...password.fieldErrors }}
        saving={profile.isPending ? 'profile' : password.isPending ? 'password' : null}
        onSaveProfile={(values) => profile.mutate({ params: { query: scope }, body: values })}
        onChangePassword={(values) => password.mutate({ params: { query: scope }, body: values })}
        onRevokeSession={(id) => revoke.mutate({ params: { path: { id }, query: scope } })}
        onLogoutAll={() => logoutAll.mutate({})}
      />
    ),
    smtp: (
      <SmtpTabView
        // Same reason as the company form: the fields are seeded from the response.
        key={smtp.data?.updated_at ?? 'loading'}
        settings={smtp.data ?? null}
        fieldErrors={saveSmtp.fieldErrors}
        saving={saveSmtp.isPending}
        testing={testSmtp.isPending}
        onSave={(values) => saveSmtp.mutate({ params: { query: scope }, body: values })}
        onTest={(to) => testSmtp.mutate({ params: { query: scope }, body: { to } })}
      />
    ),
    integrations: (
      <IntegrationsTabView
        definitions={definitions.data?.items ?? []}
        loading={definitions.isLoading}
        saving={createDefinition.isPending || updateDefinition.isPending}
        fieldErrors={{ ...createDefinition.fieldErrors, ...updateDefinition.fieldErrors }}
        onSave={(draft) => {
          const endpoints = Object.fromEntries(draft.endpoints.filter((e) => e.url).map((e) => [e.key, e.url]));
          if (draft.id) {
            updateDefinition.mutate({ params: { path: { id: draft.id } }, body: { subtype: draft.subtype, endpoints } });
          } else {
            createDefinition.mutate({ body: { provider: draft.provider, subtype: draft.subtype, endpoints } });
          }
        }}
        onDelete={(definition) => deleteDefinition.mutate({ params: { path: { id: definition.id } } })}
      />
    ),
    company: slots?.company ?? <CompanyPanel />,
    buildings: slots?.buildings ?? <BuildingsPanel />,
    plants: slots?.plants ?? <PlantsPanel />,
    analyzers: slots?.analyzers ?? null,
    users: slots?.users ?? null,
  };

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />
      <Tabs
        value={active}
        onValueChange={(tab) => {
          const next = new URLSearchParams(params.toString());
          next.set('tab', tab);
          router.replace(`${pathname}?${next.toString()}`);
        }}
        items={visible.map((id) => ({ value: id, label: t(`tabs.${id}`), content: content[id] }))}
      />
    </div>
  );
}
