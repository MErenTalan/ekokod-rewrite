'use client';

import { $api } from '@/lib/api/query';
import { useScopeParams } from '@/lib/selection/selection-store';

import type { ReportKind, ReportSelection } from './report-selection';

/** GET /reports/preview for a selection; idle until it names a building and a period. */
export function useReportPreview(kind: ReportKind, sel: ReportSelection) {
  const scope = useScopeParams();
  const ready = sel.buildingIds.length > 0 && sel.period !== null;
  return $api.useQuery(
    'get',
    '/api/v1/reports/preview',
    {
      params: {
        query: {
          ...scope,
          type: kind,
          period: sel.period ?? '',
          plant_selection: sel.plantSelection,
          building_ids: sel.buildingIds,
          plant_ids: sel.plantIds.length ? sel.plantIds : undefined,
        },
      },
    },
    { enabled: ready },
  );
}
