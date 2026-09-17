'use client';

import { X } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useEffect, useRef } from 'react';

import { formatQuantity } from '@/lib/format';

import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { IconButton } from '../ui/icon-button';
import { useAnnounce } from '../ui/live-announcer';
import { ProgressBar } from '../ui/progress-bar';

export type JobView = {
  id: string;
  label: string;
  status: 'queued' | 'running' | 'succeeded' | 'failed';
  progress?: number | null;
  message?: string;
  resultAction?: { label: string; onClick: () => void };
};

const tones = { queued: 'info', running: 'info', succeeded: 'success', failed: 'danger' } as const;

/** A background job and its result (07 §6); status changes are announced, failures assertively (07 §9). */
export function JobStatusBanner({ job, onDismiss }: { job: JobView | null; onDismiss?: () => void }) {
  const t = useTranslations('domain.job');
  const common = useTranslations('common');
  const announce = useAnnounce();
  const previous = useRef<JobView['status'] | null>(job?.status ?? null);
  const status = job?.status ?? null;
  const line = job ? t('statusLine', { label: job.label, status: t(job.status) }) : '';
  useEffect(() => {
    if (status && previous.current && previous.current !== status) announce(line, status === 'failed' ? 'assertive' : 'polite');
    previous.current = status;
  }, [status, line, announce]);
  if (!job) return null;
  return (
    <Alert
      tone={tones[job.status]}
      title={line}
      action={
        job.resultAction || onDismiss ? (
          <>
            {job.resultAction ? (
              <Button size="sm" variant="secondary" onClick={job.resultAction.onClick}>
                {job.resultAction.label}
              </Button>
            ) : null}
            {onDismiss ? <IconButton label={common('close')} icon={X} size="sm" tooltip={false} onClick={onDismiss} /> : null}
          </>
        ) : undefined
      }
    >
      {job.status === 'running' || job.message ? (
        <div className="flex flex-col gap-2">
          {job.status === 'running' ? (
            <ProgressBar
              label={t('progress', { label: job.label })}
              value={job.progress ?? null}
              valueText={job.progress == null ? undefined : formatQuantity(String(job.progress), 'percent')}
            />
          ) : null}
          {job.message ? <p>{job.message}</p> : null}
        </div>
      ) : null}
    </Alert>
  );
}
