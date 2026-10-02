'use client';

import { useQueryClient } from '@tanstack/react-query';
import { useTranslations } from 'next-intl';
import { useEffect, useMemo, useState } from 'react';

import type { JobView } from '@/components/domain/job-status-banner';
import { $api } from '@/lib/api/query';
import type { components } from '@/lib/api/schema';
import { useScopeParams } from '@/lib/selection/selection-store';

const POLL_MS = 2000;
// A backfill's fetch tasks can run for many minutes after the backfill itself.
const GIVE_UP_MS = 20 * 60 * 1000;
// The aggregates behind a finished fetch are refreshed by a follow-up task.
const SETTLE_MS = 8000;

type Children = NonNullable<components['schemas']['Job']['children']>;

/** Progress of a parent job's fetch tasks: done of total, and the overall tone. */
export function childProgress(c: Children): {
  done: number;
  total: number;
  status: JobView['status'];
} {
  const done = c.succeeded + c.partial + c.failed;
  const total = Math.max(c.enqueued ?? done + c.running, done + c.running);
  if (done < total) return { done, total, status: 'running' };
  if (c.failed > 0 && c.succeeded + c.partial === 0) return { done, total, status: 'failed' };
  return { done, total, status: 'succeeded' };
}

/** A finished parent that queued nothing: the provider has no such data to pull. */
export function nothingToFetch(c: Children): boolean {
  return childProgress(c).total === 0 && c.already_queued === 0;
}

const TONE: Record<string, JobView['status']> = {
  queued: 'queued',
  running: 'running',
  succeeded: 'succeeded',
  failed: 'failed',
};

/**
 * Polls GET /jobs/{id} until it ends (R192) and gives up after five minutes, so
 * a worker that never runs leaves a neutral message instead of a spinner for
 * ever. A 404 means the job is gone or was never ours: that is a failure to the
 * screen, never a retry loop.
 */
export function useJob(
  jobId: string | null,
  label: string,
): { job: JobView | null; errorCode?: string } {
  const t = useTranslations('domain.job');
  const scope = useScopeParams();
  const queryClient = useQueryClient();
  const [startedAt, setStartedAt] = useState<number | null>(null);
  const [expired, setExpired] = useState(false);

  useEffect(() => {
    setStartedAt(jobId ? Date.now() : null);
    setExpired(false);
  }, [jobId]);

  const query = $api.useQuery(
    'get',
    '/api/v1/jobs/{id}',
    { params: { path: { id: jobId ?? '' }, query: scope } },
    {
      enabled: Boolean(jobId) && !expired,
      refetchInterval: (q) => {
        const data = q.state.data as { status?: string; children?: Children } | undefined;
        if (data?.children && data.status === 'succeeded')
          return childProgress(data.children).status === 'running' ? POLL_MS : false;
        return data?.status === 'succeeded' || data?.status === 'failed' ? false : POLL_MS;
      },
    },
  );

  useEffect(() => {
    if (!startedAt || expired) return;
    const timer = setTimeout(
      () => setExpired(true),
      Math.max(0, startedAt + GIVE_UP_MS - Date.now()),
    );
    return () => clearTimeout(timer);
  }, [startedAt, expired]);

  const result = useMemo((): { job: JobView | null; errorCode?: string } => {
    if (!jobId) return { job: null };
    if (expired)
      return { job: { id: jobId, label, status: 'running', message: t('stillRunning') } };
    if (query.error) return { job: { id: jobId, label, status: 'failed' } };
    const status = TONE[query.data?.status ?? 'queued'] ?? 'queued';
    const children = query.data?.children;
    if (children && status === 'succeeded') {
      // "0 of 0 finished" reads as success; say that nothing was there to pull.
      if (nothingToFetch(children)) return { job: { id: jobId, label, status, message: t('nothingToFetch') } };
      const p = childProgress(children);
      return {
        job: {
          id: jobId,
          label,
          status: p.status,
          progress: p.total ? Math.round((p.done / p.total) * 100) : null,
          message: t('children', {
            done: p.done,
            total: p.total,
            succeeded: children.succeeded + children.partial,
            failed: children.failed,
            already: children.already_queued,
          }),
          errors: children.errors.slice(0, 5),
        },
      };
    }
    return {
      job: {
        id: jobId,
        label,
        status,
        message: status === 'succeeded' ? t('refreshQueued') : undefined,
        errors: children?.errors.slice(0, 5),
      },
      // R237's closed code set, for a screen that must name the data
      // condition that stopped the job (§7.10's sentences).
      errorCode: query.data?.error_code,
    };
  }, [jobId, label, expired, query.error, query.data, t]);

  // A finished job changed what the screen shows, and the queries were last
  // invalidated when it was only queued — refetch them now, and once more
  // when the follow-up aggregate refresh has had time to land.
  const finished = Boolean(jobId) && !expired && result.job?.status === 'succeeded';
  useEffect(() => {
    if (!finished) return;
    const refetch = () =>
      void queryClient.invalidateQueries({
        predicate: (q) => !JSON.stringify(q.queryKey).includes('/api/v1/jobs/'),
      });
    refetch();
    const timer = setTimeout(refetch, SETTLE_MS);
    return () => clearTimeout(timer);
  }, [finished, jobId, queryClient]);

  return result;
}
