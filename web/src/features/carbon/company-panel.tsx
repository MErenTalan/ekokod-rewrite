'use client';

import { Skeleton } from '@/components/ui/skeleton';
import { $api } from '@/lib/api/query';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { CompanyView } from './company-view';

/** Q-F10: A CA CR read the company record; other roles see the session's company name. */
export function CompanyPanel() {
  const { me, can } = useSession();
  const scope = useScopeParams();
  const readable = can('settings.company');
  const companyId = scope.company_id ?? me.company.id;
  const company = $api.useQuery('get', '/api/v1/companies/{id}', { params: { path: { id: companyId }, query: scope } }, { enabled: readable });
  if (readable && !company.data) return <Skeleton className="h-48 w-full" />;
  return <CompanyView company={readable ? company.data : undefined} companyName={company.data?.name ?? me.company.name} canEdit={can('settings.company.edit')} />;
}
