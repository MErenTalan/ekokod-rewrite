'use client';

import { useTranslations } from 'next-intl';

import { Skeleton } from '@/components/ui/skeleton';
import { downloadFile } from '@/lib/api/download';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import { istanbulToday } from '@/lib/dates';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { ReportForm } from './report-form';
import { ReportHistory } from './report-history';

/** R326's data: generate a report for the building and list its history. */
export function ReportingPanel({ buildingId }: { buildingId: string }) {
  const t = useTranslations('carbon');
  const { can } = useSession();
  const scope = useScopeParams();
  const today = istanbulToday();
  const list = $api.useInfiniteQuery(
    'get',
    '/api/v1/carbon/reports',
    { params: { query: { ...scope, building_id: buildingId, limit: 50 } } },
    { pageParamName: 'cursor', initialPageParam: '', getNextPageParam: (last: { next_cursor?: string | null }) => last.next_cursor ?? undefined },
  );
  const create = useApiMutation('post', '/api/v1/carbon/reports', { success: t('reporting.generated'), invalidate: ['/api/v1/carbon/reports'] });
  if (!list.data) return <Skeleton className="h-64 w-full" />;
  return (
    <div className="flex flex-col gap-6">
      {can('carbon.edit') ? (
        <ReportForm
          key={buildingId}
          today={today}
          initialRange={{ from: `${today.slice(0, 4)}-01-01`, to: today }}
          saving={create.isPending}
          errors={create.fieldErrors}
          onSubmit={(req) => create.mutate({ params: { query: scope }, body: { ...req, building_id: buildingId } })}
        />
      ) : (
        <p className="text-foreground-muted type-body">{t('reporting.readOnly')}</p>
      )}
      <ReportHistory
        rows={list.data.pages.flatMap((p) => p.items)}
        hasMore={list.hasNextPage}
        loadingMore={list.isFetchingNextPage}
        onLoadMore={() => void list.fetchNextPage()}
        onDownload={(r) => void downloadFile(`/api/v1/carbon/reports/${r.id}/pdf`, scope, `${r.name}.pdf`)}
      />
    </div>
  );
}
