'use client';

import { useQueries } from '@tanstack/react-query';

import { $api } from '@/lib/api/query';
import { useScopeParams } from '@/lib/selection/selection-store';

import type { JobState } from './report-actions';

const POLL_MS = 2000;

/**
 * Watches several jobs at once — one report.generate per building (E-3).
 * A 404 means the job is gone or was never ours: that is a failure, not a
 * retry loop (the same rule as useJob).
 */
export function useReportJobs(jobIds: string[]): Record<string, { status: JobState; errorCode?: string }> {
  const scope = useScopeParams();
  const results = useQueries({
    queries: jobIds.map((id) => ({
      ...$api.queryOptions('get', '/api/v1/jobs/{id}', { params: { path: { id }, query: scope } }),
      retry: false,
      refetchInterval: (q: { state: { data?: { status?: string } } }) =>
        q.state.data?.status === 'succeeded' || q.state.data?.status === 'failed' ? false : POLL_MS,
    })),
  });
  const out: Record<string, { status: JobState; errorCode?: string }> = {};
  jobIds.forEach((id, i) => {
    const r = results[i];
    if (r.isError) out[id] = { status: 'failed' };
    else out[id] = { status: (r.data?.status as JobState | undefined) ?? 'queued', errorCode: r.data?.error_code };
  });
  return out;
}
