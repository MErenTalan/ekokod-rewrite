'use client';

import { Skeleton } from '@/components/ui/skeleton';
import { downloadFile } from '@/lib/api/download';
import { $api } from '@/lib/api/query';
import { useScopeParams } from '@/lib/selection/selection-store';

import { ChecklistView } from './checklist-view';
import { SubClausePanel } from './sub-clause-panel';

/** R345's data: the clause text and the templates; each sub-clause loads its own notes and files. */
export function ChecklistPanel({ buildingId }: { buildingId: string }) {
  const scope = useScopeParams();
  const clauses = $api.useQuery('get', '/api/v1/iso50001/clauses', { params: { query: scope } });
  const templates = $api.useQuery('get', '/api/v1/iso50001/templates', { params: { query: scope } });
  if (!clauses.data) return <Skeleton className="h-64 w-full" />;
  return (
    <ChecklistView
      key={buildingId}
      clauses={clauses.data}
      templates={templates.data?.items ?? []}
      onTemplate={(tpl) => void downloadFile(`/api/v1/iso50001/templates/${tpl.id}`, scope, tpl.file_name)}
      renderSub={(sub) => <SubClausePanel buildingId={buildingId} clause={sub.id} />}
    />
  );
}
