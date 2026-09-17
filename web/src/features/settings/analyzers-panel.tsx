'use client';

import { useSearchParams } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useMemo, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { Select } from '@/components/ui/select';
import { useJob } from '@/features/jobs/use-job';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import type { Analyzer } from '@/lib/api/types';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { ALL, AnalyzersTabView, UNASSIGNED, type AnalyzerFilters } from './analyzers-tab';

/** The Analyzers tab's data (01 §7.15): filters, assignment and manual refresh. */
export function AnalyzersPanel() {
  const t = useTranslations('settings.analyzers');
  const refreshText = useTranslations('domain.refresh');
  const common = useTranslations('common');
  const { can } = useSession();
  const scope = useScopeParams();
  const params = useSearchParams();
  const [filters, setFilters] = useState<AnalyzerFilters>({
    q: '',
    // The discovery toast links here with ?unassigned=1.
    buildingId: params.get('unassigned') ? UNASSIGNED : ALL,
    provider: ALL,
  });
  const [assigning, setAssigning] = useState<{ analyzer: Analyzer; buildingId: string | null } | null>(null);
  const [startedJob, setStartedJob] = useState<{ id: string; label: string } | null>(null);

  const query = {
    ...scope,
    limit: 500,
    ...(filters.q ? { q: filters.q } : {}),
    ...(filters.buildingId === UNASSIGNED
      ? { unassigned: true }
      : filters.buildingId !== ALL
        ? { building_id: filters.buildingId }
        : {}),
    ...(filters.provider !== ALL ? { provider: [filters.provider] } : {}),
  };
  const analyzers = $api.useQuery('get', '/api/v1/analyzers', { params: { query } });
  const buildings = $api.useQuery('get', '/api/v1/buildings', { params: { query: { ...scope, limit: 500 } } });
  const update = useApiMutation('patch', '/api/v1/analyzers/{id}', { success: t('assigned'), invalidate: ['/api/v1/analyzers'] });
  const refresh = useApiMutation('post', '/api/v1/analyzers/{id}/refresh');
  const { job } = useJob(startedJob?.id ?? null, startedJob?.label ?? '');

  const providers = useMemo(
    () => [...new Set((analyzers.data?.items ?? []).map((a) => a.provider))],
    [analyzers.data],
  );

  return (
    <>
      <AnalyzersTabView
        analyzers={analyzers.data?.items ?? []}
        buildings={buildings.data?.items ?? []}
        providers={providers}
        filters={filters}
        onFiltersChange={setFilters}
        canEdit={can('settings.analyzers.edit')}
        canRefresh={can('analyzers.refresh')}
        loading={analyzers.isLoading}
        job={job}
        onDismissJob={() => setStartedJob(null)}
        onAssign={(analyzer) => setAssigning({ analyzer, buildingId: analyzer.building_id ?? null })}
        onRefresh={(analyzer, mode) =>
          refresh.mutate(
            { params: { path: { id: analyzer.id }, query: scope }, body: { mode } },
            { onSuccess: (data) => setStartedJob({ id: (data as { job_id: string }).job_id, label: refreshText(mode) }) },
          )
        }
      />

      <Dialog
        open={assigning !== null}
        onOpenChange={(open) => !open && setAssigning(null)}
        title={t('assignTitle')}
        footer={
          <Button
            loading={update.isPending}
            onClick={() => {
              if (!assigning) return;
              update.mutate({
                params: { path: { id: assigning.analyzer.id }, query: scope },
                body: assigning.buildingId ? { building_id: assigning.buildingId } : { unassign_building: true },
              });
              setAssigning(null);
            }}
          >
            {common('save')}
          </Button>
        }
      >
        {assigning ? (
          <Select
            label={t('building')}
            options={(buildings.data?.items ?? []).map((b) => ({ value: b.id, label: b.name }))}
            value={assigning.buildingId}
            placeholder={t('unassign')}
            onValueChange={(buildingId) => setAssigning({ ...assigning, buildingId })}
          />
        ) : null}
      </Dialog>
    </>
  );
}
