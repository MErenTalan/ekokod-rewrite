'use client';

import type { LoadProfiles } from '@/lib/api/types';

import { ProfileChart } from './profile-chart';

/** The Daily tab: the weekday and weekend curves side by side (01 §7.4). */
export function DailyTabView({ data, loading = false }: { data: LoadProfiles | null; loading?: boolean }) {
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <ProfileChart data={data} keys={['weekday']} loading={loading} />
      <ProfileChart data={data} keys={['weekend']} loading={loading} />
    </div>
  );
}
