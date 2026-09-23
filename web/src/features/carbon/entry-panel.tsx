'use client';

import { useTranslations } from 'next-intl';
import { useEffect, useMemo, useState } from 'react';

import { Skeleton } from '@/components/ui/skeleton';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import { istanbulToday } from '@/lib/dates';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { ActivityDialog } from './activity-dialog';
import { EntryCards, type EntrySub } from './entry-cards';
import type { CarbonTab } from './labels';

const COUNT_LIMIT = 500;

/** R324's data: declared sub-categories, their record counts and the entry dialog. */
export function EntryPanel({ buildingId, go }: { buildingId: string; go: (tab: CarbonTab) => void }) {
  const t = useTranslations('carbon');
  const { can } = useSession();
  const scope = useScopeParams();
  const [adding, setAdding] = useState<EntrySub | null>(null);
  // Review Focus 4: a building switch never carries an open entry over.
  useEffect(() => setAdding(null), [buildingId]);

  const building = { ...scope, building_id: buildingId };
  const catalogue = $api.useQuery('get', '/api/v1/carbon/activity-catalogue', { params: { query: scope } });
  const selected = $api.useQuery('get', '/api/v1/carbon/selected-activities', { params: { query: building } });
  const activities = $api.useQuery('get', '/api/v1/carbon/activities', { params: { query: { ...building, limit: COUNT_LIMIT } } });
  const factors = $api.useQuery(
    'get',
    '/api/v1/carbon/emission-factors',
    { params: { query: { ...scope, sub_category: adding?.key ?? '' } } },
    { enabled: adding !== null },
  );
  const create = useApiMutation('post', '/api/v1/carbon/activities', {
    success: t('dialog.saved'),
    invalidate: ['/api/v1/carbon/activities', '/api/v1/carbon/overview'],
  });

  const subs = useMemo<EntrySub[]>(() => {
    const keys = new Set(selected.data?.activity_keys ?? []);
    return (catalogue.data?.items ?? []).flatMap((m) => m.subs.filter((s) => keys.has(s.key)).map((s) => ({ ...s, main: m.key })));
  }, [catalogue.data, selected.data]);
  const counts = useMemo(() => {
    const out: Record<string, number> = {};
    for (const a of activities.data?.items ?? []) out[a.sub_category] = (out[a.sub_category] ?? 0) + 1;
    return out;
  }, [activities.data]);

  if (!catalogue.data || !selected.data || !activities.data) return <Skeleton className="h-64 w-full" />;
  return (
    <>
      <EntryCards
        subs={subs}
        counts={counts}
        full={activities.data.next_cursor != null}
        editable={can('carbon.edit')}
        onAdd={setAdding}
        onGoSelection={() => go('selection')}
      />
      {adding ? (
        <ActivityDialog
          open
          sub={adding}
          factors={factors.data?.items ?? []}
          today={istanbulToday()}
          errors={create.fieldErrors}
          saving={create.isPending}
          onClose={() => setAdding(null)}
          onSubmit={(entry) =>
            create.mutate({ params: { query: scope }, body: { ...entry, building_id: buildingId } }, { onSuccess: () => setAdding(null) })
          }
        />
      ) : null}
    </>
  );
}
