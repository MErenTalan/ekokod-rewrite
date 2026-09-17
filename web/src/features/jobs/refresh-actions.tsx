'use client';

import { RefreshCw, Zap } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { JobStatusBanner, type JobView } from '@/components/domain/job-status-banner';
import { Button } from '@/components/ui/button';
import { useApiMutation } from '@/lib/api/mutation';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { useJob } from './use-job';

export type RefreshMode = 'hourly' | 'energy';

export type RefreshActionsViewProps = {
  /** Null disables both buttons and explains why (no analyzer chosen yet). */
  analyzerId: string | null;
  pending: boolean;
  job: JobView | null;
  onRefresh: (mode: RefreshMode) => void;
  onDismiss: () => void;
};

/** The two on-demand pull buttons and the state of the job they started. */
export function RefreshActionsView({ analyzerId, pending, job, onRefresh, onDismiss }: RefreshActionsViewProps) {
  const t = useTranslations('domain.refresh');
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="secondary" iconStart={RefreshCw} disabled={!analyzerId} loading={pending} onClick={() => onRefresh('hourly')}>
          {t('hourly')}
        </Button>
        <Button variant="secondary" iconStart={Zap} disabled={!analyzerId} loading={pending} onClick={() => onRefresh('energy')}>
          {t('energy')}
        </Button>
        {!analyzerId ? <p className="text-foreground-muted type-small">{t('chooseAnalyzer')}</p> : null}
      </div>
      <JobStatusBanner job={job} onDismiss={onDismiss} />
    </div>
  );
}

/**
 * "Refresh hourly values" and "Refresh energy values" (01 §7.2, §7.3): both
 * enqueue an on-demand pull (R164) and then poll the job (R192). Hidden
 * entirely from a role that may not refresh.
 */
export function RefreshActions({ analyzerId }: { analyzerId: string | null }) {
  const t = useTranslations('domain.refresh');
  const { can } = useSession();
  const scope = useScopeParams();
  const [started, setStarted] = useState<{ id: string; mode: RefreshMode } | null>(null);
  const refresh = useApiMutation('post', '/api/v1/analyzers/{id}/refresh');
  const { job } = useJob(started?.id ?? null, started ? t(started.mode) : '');

  if (!can('analyzers.refresh')) return null;

  return (
    <RefreshActionsView
      analyzerId={analyzerId}
      pending={refresh.isPending}
      job={job}
      onDismiss={() => setStarted(null)}
      onRefresh={(mode) => {
        if (!analyzerId) return;
        refresh.mutate(
          { params: { path: { id: analyzerId }, query: scope }, body: { mode } },
          { onSuccess: (data) => setStarted({ id: (data as { job_id: string }).job_id, mode }) },
        );
      }}
    />
  );
}
