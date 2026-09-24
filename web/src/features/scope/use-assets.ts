'use client';

import { useMemo } from 'react';

import { $api } from '@/lib/api/query';
import type { Analyzer, Building } from '@/lib/api/types';
import { useScopeParams, useSelection } from '@/lib/selection/selection-store';

const LIMIT = 500;
const collator = new Intl.Collator('tr');

export type Assets = {
  buildings: Building[];
  analyzers: Analyzer[];
  /** The analyzers of the selected building, in name order. */
  buildingAnalyzers: Analyzer[];
  loading: boolean;
};

export const analyzerLabel = (a: Analyzer): string => a.customer_name?.trim() || a.installation_number;

/**
 * Every screen's buildings and analyzers, scoped for admins (R139) and sorted
 * the way Turkish readers expect. One query pair, shared by the picker, the
 * dashboard map and the settings tabs.
 */
export function useAssets(): Assets {
  const scope = useScopeParams();
  const { buildingId } = useSelection();
  const buildings = $api.useQuery('get', '/api/v1/buildings', {
    params: { query: { ...scope, include: ['analyzer_count', 'active_status'], limit: LIMIT } },
  });
  const analyzers = $api.useQuery('get', '/api/v1/analyzers', { params: { query: { ...scope, limit: LIMIT } } });

  return useMemo(() => {
    const sortedBuildings = [...(buildings.data?.items ?? [])].sort((a, b) => collator.compare(a.name, b.name));
    const sortedAnalyzers = [...(analyzers.data?.items ?? [])].sort((a, b) =>
      collator.compare(analyzerLabel(a), analyzerLabel(b)),
    );
    return {
      buildings: sortedBuildings,
      analyzers: sortedAnalyzers,
      buildingAnalyzers: sortedAnalyzers.filter((a) => a.building_id === buildingId),
      loading: buildings.isLoading || analyzers.isLoading,
    };
  }, [buildings.data, analyzers.data, buildings.isLoading, analyzers.isLoading, buildingId]);
}
