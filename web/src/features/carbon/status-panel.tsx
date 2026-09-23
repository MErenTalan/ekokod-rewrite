'use client';

import { useLocale, useTranslations } from 'next-intl';
import { useEffect, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import type { Locale } from '@/i18n/locale';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import type { CarbonActivity } from '@/lib/api/types';
import { istanbulToday } from '@/lib/dates';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { ActivityDialog } from './activity-dialog';
import { subKey } from './labels';
import { periodLabel, StatusTable, type StatusFilters } from './status-table';

const INVALIDATE = ['/api/v1/carbon/activities', '/api/v1/carbon/overview'];

/** R325's data: the building's records, paged, with approval, edit and delete. */
export function StatusPanel({ buildingId }: { buildingId: string }) {
  const t = useTranslations('carbon');
  const locale = useLocale() as Locale;
  const { can } = useSession();
  const scope = useScopeParams();
  const [filters, setFilters] = useState<StatusFilters>({ status: 'all', scope: 'all' });
  const [editing, setEditing] = useState<CarbonActivity | null>(null);
  const [deleting, setDeleting] = useState<CarbonActivity | null>(null);
  useEffect(() => {
    setEditing(null);
    setDeleting(null);
  }, [buildingId]);

  const list = $api.useInfiniteQuery(
    'get',
    '/api/v1/carbon/activities',
    {
      params: {
        query: {
          ...scope,
          building_id: buildingId,
          limit: 100,
          status: filters.status === 'all' ? undefined : filters.status,
          scope: filters.scope === 'all' ? undefined : filters.scope,
        },
      },
    },
    { pageParamName: 'cursor', initialPageParam: '', getNextPageParam: (last: { next_cursor?: string | null }) => last.next_cursor ?? undefined },
  );
  const catalogue = $api.useQuery('get', '/api/v1/carbon/activity-catalogue', { params: { query: scope } });
  const factors = $api.useQuery(
    'get',
    '/api/v1/carbon/emission-factors',
    { params: { query: { ...scope, sub_category: editing?.sub_category ?? '' } } },
    { enabled: editing !== null },
  );
  const approve = useApiMutation('post', '/api/v1/carbon/activities/{id}/status', { success: t('statusTab.approved'), invalidate: INVALIDATE });
  const reject = useApiMutation('post', '/api/v1/carbon/activities/{id}/status', { success: t('statusTab.rejected'), invalidate: INVALIDATE });
  const update = useApiMutation('patch', '/api/v1/carbon/activities/{id}', { success: t('statusTab.saved'), invalidate: INVALIDATE });
  const remove = useApiMutation('delete', '/api/v1/carbon/activities/{id}', { success: t('statusTab.deleted'), invalidate: INVALIDATE });

  const setStatus = (a: CarbonActivity, next: 'approved' | 'rejected') =>
    (next === 'approved' ? approve : reject).mutate({ params: { path: { id: a.id }, query: scope }, body: { status: next } });
  const sub = editing ? catalogue.data?.items.flatMap((m) => m.subs).find((s) => s.key === editing.sub_category) : undefined;

  if (!list.data) return <Skeleton className="h-64 w-full" />;
  return (
    <>
      <StatusTable
        rows={list.data.pages.flatMap((p) => p.items)}
        editable={can('carbon.edit')}
        filters={filters}
        onFilters={setFilters}
        onApprove={(a) => setStatus(a, 'approved')}
        onReject={(a) => setStatus(a, 'rejected')}
        onEdit={setEditing}
        onDelete={setDeleting}
        hasMore={list.hasNextPage}
        loadingMore={list.isFetchingNextPage}
        onLoadMore={() => void list.fetchNextPage()}
      />
      {editing && sub ? (
        <ActivityDialog
          open
          sub={sub}
          initial={editing}
          factors={factors.data?.items ?? []}
          today={istanbulToday()}
          errors={update.fieldErrors}
          saving={update.isPending}
          onClose={() => setEditing(null)}
          onSubmit={({ sub_category, ...entry }) =>
            update.mutate(
              { params: { path: { id: editing.id }, query: scope }, body: { ...entry, sub_category } },
              { onSuccess: () => setEditing(null) },
            )
          }
        />
      ) : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
        title={t('statusTab.deleteTitle')}
        footer={
          <Button
            variant="danger"
            onClick={() => {
              if (deleting) remove.mutate({ params: { path: { id: deleting.id }, query: scope } });
              setDeleting(null);
            }}
          >
            {t('statusTab.delete')}
          </Button>
        }
      >
        {deleting ? <p>{t('statusTab.deleteConfirm', { sub: t(subKey(deleting.sub_category)), period: periodLabel(deleting, locale) })}</p> : null}
      </Dialog>
    </>
  );
}
