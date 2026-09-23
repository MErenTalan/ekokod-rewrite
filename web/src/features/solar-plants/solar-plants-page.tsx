'use client';

import Link from 'next/link';
import { useTranslations } from 'next-intl';
import { useEffect, useMemo, useState } from 'react';

import { JobStatusBanner } from '@/components/domain/job-status-banner';
import { PageHeader } from '@/components/shell/page-header';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { EmptyState } from '@/components/ui/empty-state';
import { Select } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs } from '@/components/ui/tabs';
import type { DateRange } from '@/components/ui/date-range-picker';
import { downloadFile } from '@/lib/api/download';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { useJob } from '../jobs/use-job';
import { ALARM_PAGE, AlarmsTabView } from './alarms-tab';
import { ConnectionStatus } from './connection-status';
import { DevicesTabView } from './devices-tab';
import { rangeTooLong } from './format';
import { HistoryTabView, type Granularity } from './history-tab';
import { OverviewTabView } from './overview-tab';

/** Today's Istanbul calendar date, YYYY-MM-DD, whatever the browser's zone (R161). */
function istanbulToday(): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Istanbul' }).format(new Date());
}

function shiftDays(date: string, days: number): string {
  const d = new Date(`${date}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

/** The solar screen of 01 §7.7: one linked plant at a time, four tabs. */
export function SolarPlantsPage() {
  const t = useTranslations('solarPlants');
  const { can } = useSession();
  const scope = useScopeParams();
  const today = istanbulToday();
  const tomorrow = shiftDays(today, 1);

  const plants = $api.useQuery('get', '/api/v1/power-plants', { params: { query: { ...scope, limit: 500 } } });
  const linked = useMemo(() => (plants.data?.items ?? []).filter((p) => p.isolar_ps_id), [plants.data]);
  const [plantId, setPlantId] = useState<string | null>(null);
  useEffect(() => {
    if (!plantId && linked.length > 0) setPlantId(linked[0].id);
  }, [linked, plantId]);

  const id = plantId ?? '';
  const enabled = Boolean(plantId);
  const path = { id };
  const realtime = $api.useQuery('get', '/api/v1/plants/{id}/realtime', { params: { path, query: scope } }, { enabled });
  const revenue = $api.useQuery('get', '/api/v1/plants/{id}/revenue', { params: { path, query: scope } }, { enabled });
  const monthDaily = $api.useQuery('get', '/api/v1/plants/{id}/production',
    { params: { path, query: { ...scope, granularity: 'day', from: `${today.slice(0, 8)}01`, to: tomorrow } } }, { enabled });
  const history = $api.useQuery('get', '/api/v1/plants/{id}/production',
    { params: { path, query: { ...scope, granularity: 'month', from: `${Number(today.slice(0, 4)) - 1}${today.slice(4, 8)}01`, to: tomorrow } } },
    { enabled });

  const [query, setQuery] = useState('');
  const devices = $api.useQuery('get', '/api/v1/plants/{id}/devices', { params: { path, query: { ...scope, q: query || undefined } } }, { enabled });

  const [granularity, setGranularity] = useState<Granularity>('day');
  const [range, setRange] = useState<DateRange>({ from: shiftDays(today, -30), to: tomorrow });
  const historyOk = !rangeTooLong(granularity, range.from, range.to);
  const series = $api.useQuery('get', '/api/v1/plants/{id}/production',
    { params: { path, query: { ...scope, granularity, from: range.from, to: range.to } } }, { enabled: enabled && historyOk });

  const alarms = $api.useInfiniteQuery('get', '/api/v1/plants/{id}/alarms',
    { params: { path, query: { ...scope, limit: ALARM_PAGE } } },
    {
      enabled,
      pageParamName: 'cursor',
      initialPageParam: '',
      getNextPageParam: (last: { next_cursor?: string | null }) => last.next_cursor ?? undefined,
    });

  const [jobId, setJobId] = useState<string | null>(null);
  const sync = useApiMutation('post', '/api/v1/plants/{id}/sync');
  const { job } = useJob(jobId, t('syncJob'));
  useEffect(() => {
    if (job?.status === 'succeeded') {
      void realtime.refetch();
      void revenue.refetch();
      void monthDaily.refetch();
    }
    // Refetch once per finished job, not on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [job?.status]);

  if (plants.isPending) return <Skeleton className="h-64 w-full" />;
  if (linked.length === 0) {
    return (
      <div className="flex flex-col gap-6">
        <PageHeader title={t('title')} description={t('subtitle')} />
        <EmptyState title={t('noLinked')} description={t('noLinkedDescription')}
          action={<Link className="text-primary underline underline-offset-4" href="/ekorm/settings?tab=plants">{t('goToSettings')}</Link>} />
      </div>
    );
  }

  const pages = alarms.data?.pages ?? [];
  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />
      <Card className="flex flex-wrap items-end justify-between gap-4">
        <div className="w-full max-w-xs">
          <Select label={t('plant')} value={id} onValueChange={setPlantId}
            options={linked.map((p) => ({ value: p.id, label: p.name }))} />
        </div>
        <ConnectionStatus realtime={realtime.data} />
        {can('plants.manage') ? (
          <Button onClick={() => sync.mutate({ params: { path, query: scope } }, { onSuccess: (data) => setJobId(data.job_id) })}
            disabled={sync.isPending || job?.status === 'queued' || job?.status === 'running'}>
            {t('sync')}
          </Button>
        ) : null}
      </Card>
      <JobStatusBanner job={job} onDismiss={() => setJobId(null)} />
      <Tabs
        defaultValue="overview"
        items={[
          { value: 'overview', label: t('tabs.overview'), content: (
            <OverviewTabView realtime={realtime.data} revenue={revenue.data} monthDaily={monthDaily.data} history={history.data}
              loading={realtime.isPending || monthDaily.isPending} />
          ) },
          { value: 'devices', label: t('tabs.devices'), content: (
            <DevicesTabView devices={devices.data?.items ?? []} query={query} onQueryChange={setQuery} />
          ) },
          { value: 'history', label: t('tabs.history'), content: (
            <HistoryTabView granularity={granularity} range={range} series={series.data} loading={series.isPending && historyOk}
              onGranularityChange={setGranularity} onRangeChange={setRange}
              onExport={() => void downloadFile(`/api/v1/plants/${id}/production/export`,
                { ...scope, granularity, from: range.from, to: range.to }, `uretim-${range.from}-${range.to}.xlsx`)} />
          ) },
          { value: 'alarms', label: t('tabs.alarms'), content: (
            <AlarmsTabView items={pages.flatMap((p) => p.items)} total={pages[0]?.total ?? 0} loadingMore={alarms.isFetchingNextPage}
              onLoadMore={alarms.hasNextPage ? () => void alarms.fetchNextPage() : undefined} />
          ) },
        ]}
      />
    </div>
  );
}
