'use client';

import { useLocale, useTranslations } from 'next-intl';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { JobView } from '@/components/domain/job-status-banner';
import { JobStatusBanner } from '@/components/domain/job-status-banner';
import type { JobRun } from '@/lib/api/types';
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
};

const STATUS_TONE = {
  running: 'neutral', success: 'success', partial: 'warning', failed: 'danger',
} as const;
const STATUS_LABEL = {
  running: 'jobs.statuses.running', success: 'jobs.statuses.success',
  partial: 'jobs.statuses.partial', failed: 'jobs.statuses.failed',
} as const;

/** 09 §F7's job history: what ran, over what scope, and with what result. */
export function JobRunsTableView({
  runs, triggerable, canTrigger, onTrigger, job = null, onDismissJob, loading = false,
}: JobRunsTableViewProps) {
  const t = useTranslations('messages');
  const locale = useLocale() as Locale;
  // Job types are dotted codes (alarm.evaluate); unknown ones fall back to the code itself.
  const jobLabel = (type: string) => {
    const key = `jobs.types.${type.replace(/[._](\w)/g, (_, c: string) => c.toUpperCase())}` as Parameters<typeof t>[0];
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
                  <TableCell>{formatDateTime(run.started_at, locale)}</TableCell>
                  <TableCell>{run.finished_at ? formatDateTime(run.finished_at, locale) : '—'}</TableCell>
                  <TableCell>
                    <Badge tone={STATUS_TONE[run.status as keyof typeof STATUS_TONE] ?? 'neutral'}>
                      {t(STATUS_LABEL[run.status as keyof typeof STATUS_LABEL] ?? 'jobs.statuses.running')}
                    </Badge>
                    {run.error ? (
                      <p className="text-foreground-muted type-caption">{run.error}</p>
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
