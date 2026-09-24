'use client';

import type { LoadProfiles } from '@/lib/api/types';

import { ProfileChart } from './profile-chart';
import { SEASONAL_KEYS } from './profile-label';

/** The Seasonal tab: the eight season × day-type curves, each with its note. */
export function SeasonalTabView({ data, loading = false }: { data: LoadProfiles | null; loading?: boolean }) {
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      {SEASONAL_KEYS.map((key) => (
        <ProfileChart key={key} data={data} keys={[key]} loading={loading} />
      ))}
    </div>
  );
}
