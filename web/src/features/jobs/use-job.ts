'use client';

import { useTranslations } from 'next-intl';
import { useEffect, useMemo, useState } from 'react';

import type { JobView } from '@/components/domain/job-status-banner';
import { $api } from '@/lib/api/query';
import { useScopeParams } from '@/lib/selection/selection-store';

const POLL_MS = 2000;
const GIVE_UP_MS = 5 * 60 * 1000;

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
export function useJob(jobId: string | null, label: string): { job: JobView | null; errorCode?: string } {
  const t = useTranslations('domain.job');
  const scope = useScopeParams();
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
        const status = (q.state.data as { status?: string } | undefined)?.status;
        return status === 'succeeded' || status === 'failed' ? false : POLL_MS;
      },
    },
  );

  useEffect(() => {
    if (!startedAt || expired) return;
    const timer = setTimeout(() => setExpired(true), Math.max(0, startedAt + GIVE_UP_MS - Date.now()));
    return () => clearTimeout(timer);
  }, [startedAt, expired]);

  return useMemo(() => {
    if (!jobId) return { job: null };
    if (expired) return { job: { id: jobId, label, status: 'running', message: t('stillRunning') } };
    if (query.error) return { job: { id: jobId, label, status: 'failed' } };
    const status = TONE[query.data?.status ?? 'queued'] ?? 'queued';
    return {
      job: {
        id: jobId,
        label,
        status,
        message: status === 'succeeded' ? t('refreshQueued') : undefined,
      },
      // R237's closed code set, for a screen that must name the data
      // condition that stopped the job (§7.10's sentences).
      errorCode: query.data?.error_code,
    };
  }, [jobId, label, expired, query.error, query.data, t]);
}
