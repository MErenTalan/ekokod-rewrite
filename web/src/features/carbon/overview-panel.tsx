'use client';

import { useState } from 'react';

import { Skeleton } from '@/components/ui/skeleton';
import { $api } from '@/lib/api/query';
import { istanbulToday } from '@/lib/dates';
import { useScopeParams } from '@/lib/selection/selection-store';

import type { CarbonTab } from './labels';
import { OverviewView } from './overview-view';

/** R322's data: the selected building's year. */
export function OverviewPanel({ buildingId, go }: { buildingId: string; go: (tab: CarbonTab) => void }) {
  const scope = useScopeParams();
  const thisYear = Number(istanbulToday().slice(0, 4));
  const [year, setYear] = useState(thisYear);
  const query = $api.useQuery('get', '/api/v1/carbon/overview', { params: { query: { ...scope, building_id: buildingId, year } } });
  if (!query.data) return <Skeleton className="h-64 w-full" />;
  return (
    <OverviewView
      overview={query.data}
      years={Array.from({ length: 5 }, (_, i) => thisYear - i)}
      onYearChange={setYear}
      onSeeAll={() => go('status')}
      loading={query.isFetching && query.data.year !== year}
    />
  );
}
