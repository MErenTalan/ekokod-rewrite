'use client';

import { useTranslations } from 'next-intl';
import { useEffect, useState } from 'react';

import { PageHeader } from '@/components/shell/page-header';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/search-input';
import { Select } from '@/components/ui/select';
import { Tabs } from '@/components/ui/tabs';
import { useJob } from '@/features/jobs/use-job';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import type { paths } from '@/lib/api/schema';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { JobRunsTableView } from './job-runs-table';
import { MessageTableView } from './message-table';

type OperationalMessagesQuery = NonNullable<paths['/api/v1/messages']['get']['parameters']['query']>;

const ALL = 'all';

/** The filter values the API declares, so a widget can never send a third. */
type MessageKind = NonNullable<NonNullable<OperationalMessagesQuery['kind']>>;
type MessageStatus = NonNullable<NonNullable<OperationalMessagesQuery['status']>>;
type TriggerableJob = paths['/api/v1/job-runs/{type}/trigger']['post']['parameters']['path']['type'];
const LIST_LIMIT = 200;
const SEARCH_DEBOUNCE_MS = 300;

/**
 * R220's allow-list, spelled once. It matches the server's enum by
 * construction: TestTriggerEnumMatchesTheAllowList keeps dto.JobTriggerRequest
 * and ops.Triggerable equal, and this list mirrors that enum.
 */
const TRIGGERABLE: readonly TriggerableJob[] = [
  'alarm.evaluate', 'billing.dispatch', 'integration.sync_dispatch', 'epias.sync_prices',
];

/** The unified operational log of 01 §7.13, with 09 §F7's job history beside it. */
export function MessagesPage() {
  const t = useTranslations('messages');
  const { can } = useSession();
  const scope = useScopeParams();

  const [tab, setTab] = useState('messages');
  const [search, setSearch] = useState('');
  const [debounced, setDebounced] = useState('');
  const [kind, setKind] = useState<MessageKind | typeof ALL>(ALL);
  const [status, setStatus] = useState<MessageStatus | typeof ALL>(ALL);
  const [startedJob, setStartedJob] = useState<{ id: string; label: string } | null>(null);

  const canReadRuns = can('jobs.runs.read');
  const canTrigger = can('jobs.trigger');

  // The search box types faster than the API should be asked.
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(search.trim()), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [search]);

  const messages = $api.useQuery('get', '/api/v1/messages', {
    params: {
      query: {
        ...scope,
        limit: LIST_LIMIT,
        ...(debounced ? { q: debounced } : {}),
        ...(kind === ALL ? {} : { kind }),
        ...(status === ALL ? {} : { status }),
      },
    },
  });

  // The job-runs query is only issued when the role may read it, so a building
  // admin never fires a request their token would refuse.
  const runs = $api.useQuery(
    'get',
    '/api/v1/job-runs',
    { params: { query: { ...scope, limit: LIST_LIMIT } } },
    { enabled: canReadRuns },
  );

  const trigger = useApiMutation('post', '/api/v1/job-runs/{type}/trigger', {
    success: t('jobs.triggered'),
    invalidate: ['/api/v1/job-runs'],
  });
  const { job } = useJob(startedJob?.id ?? null, startedJob?.label ?? '');

  const filtered = Boolean(debounced) || kind !== ALL || status !== ALL;

  const tabs = [
    {
      value: 'messages',
      label: t('tabs.messages'),
      content: (
        <div className="flex flex-col gap-4 pt-4">
          <div className="grid gap-3 md:grid-cols-3">
            <SearchInput
              label={t('search')}
              placeholder={t('searchPlaceholder')}
              value={search}
              onValueChange={setSearch}
            />
            <Select
              label={t('type')}
              value={kind}
              onValueChange={(value) => setKind(value as MessageKind | typeof ALL)}
              options={[
                { value: ALL, label: t('types.all') },
                { value: 'alarm', label: t('types.alarm') },
                { value: 'job', label: t('types.job') },
                { value: 'system', label: t('types.system') },
              ]}
            />
            <Select
              label={t('status')}
              value={status}
              onValueChange={(value) => setStatus(value as MessageStatus | typeof ALL)}
              options={[
                { value: ALL, label: t('statuses.all') },
                { value: 'success', label: t('statuses.success') },
                { value: 'error', label: t('statuses.error') },
                { value: 'warning', label: t('statuses.warning') },
                { value: 'info', label: t('statuses.info') },
              ]}
            />
          </div>
          <MessageTableView
            messages={messages.data?.items ?? []}
            filtered={filtered}
            loading={messages.isPending}
          />
        </div>
      ),
    },
    ...(canReadRuns
      ? [{
          value: 'jobRuns',
          label: t('tabs.jobRuns'),
          content: (
            <div className="pt-4">
              <JobRunsTableView
                runs={runs.data?.items ?? []}
                triggerable={TRIGGERABLE}
                canTrigger={canTrigger}
                loading={runs.isPending}
                job={job}
                onDismissJob={() => setStartedJob(null)}
                onTrigger={(jobType) => {
                  trigger.mutate(
                    { params: { path: { type: jobType as TriggerableJob }, query: scope } },
                    { onSuccess: (data) => setStartedJob({ id: data.job_id, label: jobType }) },
                  );
                }}
              />
            </div>
          ),
        }]
      : []),
  ];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={t('title')}
        actions={
          <Button
            variant="secondary"
            onClick={() => {
              void messages.refetch();
              if (canReadRuns) void runs.refetch();
            }}
          >
            {t('refresh')}
          </Button>
        }
      />
      <Tabs items={tabs} value={tab} onValueChange={setTab} />
    </div>
  );
}
