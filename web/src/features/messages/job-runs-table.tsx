'use client';

import { useLocale, useTranslations } from 'next-intl';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import type { JobView } from '@/components/domain/job-status-banner';
import { JobStatusBanner } from '@/components/domain/job-status-banner';
import type { JobRun } from '@/lib/api/types';
import type { components } from '@/lib/api/schema';
import { formatDateTime, formatNumber } from '@/lib/format';
import type { Locale } from '@/i18n/locale';

export type JobRunsTableViewProps = {
  runs: JobRun[];
  /** R220's allow-list, taken from the generated enum so it cannot drift. */
  triggerable: readonly string[];
  /** jobs.trigger is A only; A CA both read the list. */
  canTrigger: boolean;
  onTrigger: (jobType: string) => void;
  job?: JobView | null;
  onDismissJob?: () => void;
  loading?: boolean;
  /** Analyzer id → name, so a fetch run names its meter. */
  analyzerNames?: Record<string, string>;
  /** Queue load, admin only. */
  queues?: components['schemas']['JobQueue'][];
};

type RunScope = {
  analyzer_id?: string;
  kind?: string;
  window?: { from: string; to: string };
  from?: string;
  to?: string;
  credential_id?: string;
};

/** What the run covered: its meter, reading kind and date range. */
export function describeScope(scope: unknown, names: Record<string, string> = {}): string {
  if (!scope || typeof scope !== 'object') return '';
  const s = scope as RunScope;
  const range = s.window ?? (s.from && s.to ? { from: s.from, to: s.to } : undefined);
  return [
    s.analyzer_id ? (names[s.analyzer_id] ?? s.analyzer_id.slice(0, 8)) : null,
    s.kind ?? null,
    range ? `${range.from.slice(0, 10)} → ${range.to.slice(0, 10)}` : null,
  ]
    .filter(Boolean)
    .join(' · ');
}

const STATUS_TONE = {
  running: 'neutral',
  success: 'success',
  partial: 'warning',
  failed: 'danger',
} as const;
const STATUS_LABEL = {
  running: 'jobs.statuses.running',
  success: 'jobs.statuses.success',
  partial: 'jobs.statuses.partial',
  failed: 'jobs.statuses.failed',
} as const;

/** 09 §F7's job history: what ran, over what scope, and with what result. */
export function JobRunsTableView({
  runs,
  triggerable,
  canTrigger,
  onTrigger,
  job = null,
  onDismissJob,
  loading = false,
  analyzerNames = {},
  queues,
}: JobRunsTableViewProps) {
  const t = useTranslations('messages');
  const locale = useLocale() as Locale;
  // Job types are dotted codes (alarm.evaluate); unknown ones fall back to the code itself.
  const jobLabel = (type: string) => {
    const key =
      `jobs.types.${type.replace(/[._](\w)/g, (_, c: string) => c.toUpperCase())}` as Parameters<
        typeof t
      >[0];
    return t.has(key) ? t(key) : type;
  };

  return (
    <div className="flex flex-col gap-4">
      {canTrigger ? (
        <div className="flex flex-wrap gap-2">
          {triggerable.map((jobType) => (
            <Button key={jobType} variant="secondary" size="sm" onClick={() => onTrigger(jobType)}>
              {t('jobs.triggerLabel', { job: jobLabel(jobType) })}
            </Button>
          ))}
        </div>
      ) : null}

      {job ? <JobStatusBanner job={job} onDismiss={onDismissJob} /> : null}

      {queues?.length ? (
        <TableContainer label={t('jobs.queues.title')}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('jobs.queues.queue')}</TableHead>
                <TableHead numeric>{t('jobs.queues.pending')}</TableHead>
                <TableHead numeric>{t('jobs.queues.active')}</TableHead>
                <TableHead numeric>{t('jobs.queues.retry')}</TableHead>
                <TableHead numeric>{t('jobs.queues.archived')}</TableHead>
                <TableHead numeric>{t('jobs.queues.processedToday')}</TableHead>
                <TableHead numeric>{t('jobs.queues.failedToday')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {queues.map((q) => (
                <TableRow key={q.queue}>
                  <TableCell>
                    {q.queue}
                    {q.paused ? <Badge tone="warning">{t('jobs.queues.paused')}</Badge> : null}
                  </TableCell>
                  <TableCell numeric>{formatNumber(q.pending + q.scheduled)}</TableCell>
                  <TableCell numeric>{formatNumber(q.active)}</TableCell>
                  <TableCell numeric>{formatNumber(q.retry)}</TableCell>
                  <TableCell numeric>{formatNumber(q.archived)}</TableCell>
                  <TableCell numeric>{formatNumber(q.processed_today)}</TableCell>
                  <TableCell numeric>{formatNumber(q.failed_today)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      ) : null}

      {loading ? (
        <Skeleton className="h-64 w-full" />
      ) : runs.length === 0 ? (
        <EmptyState title={t('jobs.empty')} description={t('jobs.emptyDescription')} />
      ) : (
        <TableContainer label={t('tabs.jobRuns')}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('jobs.type')}</TableHead>
                <TableHead>{t('jobs.scope')}</TableHead>
                <TableHead>{t('jobs.startedAt')}</TableHead>
                <TableHead>{t('jobs.finishedAt')}</TableHead>
                <TableHead>{t('jobs.status')}</TableHead>
                <TableHead numeric>{t('jobs.processed')}</TableHead>
                <TableHead numeric>{t('jobs.skipped')}</TableHead>
                <TableHead numeric>{t('jobs.failed')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {runs.map((run) => (
                <TableRow key={run.id}>
                  <TableCell>
                    <p className="font-medium">{jobLabel(run.job_type)}</p>
                    <p className="text-foreground-muted type-caption">{run.job_type}</p>
                  </TableCell>
                  <TableCell>{describeScope(run.scope, analyzerNames) || '—'}</TableCell>
                  <TableCell>{formatDateTime(run.started_at, locale)}</TableCell>
                  <TableCell>
                    {run.finished_at ? formatDateTime(run.finished_at, locale) : '—'}
                  </TableCell>
                  <TableCell>
                    <Badge tone={STATUS_TONE[run.status as keyof typeof STATUS_TONE] ?? 'neutral'}>
                      {t(
                        STATUS_LABEL[run.status as keyof typeof STATUS_LABEL] ??
                          'jobs.statuses.running',
                      )}
                    </Badge>
                    {run.error ? (
                      <p className="max-w-md break-words text-danger type-caption">{run.error}</p>
                    ) : null}
                  </TableCell>
                  <TableCell numeric>{formatNumber(run.processed)}</TableCell>
                  <TableCell numeric>{formatNumber(run.skipped)}</TableCell>
                  <TableCell numeric>{formatNumber(run.failed)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </div>
  );
}
