'use client';

import { useTranslations } from 'next-intl';

import { Alert } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Dialog } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import type { AlarmEvaluation } from '@/lib/api/types';

export type EvaluateDialogViewProps = {
  open: boolean;
  alarmName: string;
  evaluation: AlarmEvaluation | null;
  onClose: () => void;
  loading?: boolean;
};

/**
 * The dry run of R223. It says plainly that nothing was sent and nothing was
 * recorded, and it renders a no-verdict as "could not be decided" — never as
 * "within limits", which is the distinction the whole evaluation preserves.
 */
export function EvaluateDialogView({ open, alarmName, evaluation, onClose, loading = false }: EvaluateDialogViewProps) {
  const t = useTranslations('alarms');

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => !next && onClose()}
      title={`${t('evaluate.title')} — ${alarmName}`}
      size="lg"
    >
      <div className="flex flex-col gap-4">
        <Alert tone="info" title={t('evaluate.noNotification')} />

        {loading || !evaluation ? (
          <Skeleton className="h-40 w-full" />
        ) : (
          <ul className="flex flex-col gap-3">
            {evaluation.analyzers.map((row) => (
              <li key={row.analyzer_id} className="flex flex-col gap-2 rounded-md border border-border p-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-foreground type-label">{row.installation_number}</span>
                  <Badge tone={row.fired ? 'warning' : 'success'}>
                    {row.fired ? t('evaluate.fired') : t('evaluate.quiet')}
                  </Badge>
                </div>

                {row.breaches.length > 0 ? (
                  <ul className="flex list-disc flex-col gap-1 ps-5">
                    {row.breaches.map((breach) => (
                      <li key={breach.field} className="text-foreground type-body">{breach.message}</li>
                    ))}
                  </ul>
                ) : null}

                {row.no_verdict.length > 0 ? (
                  <div className="flex flex-col gap-1">
                    <p className="text-foreground type-body">
                      {t('evaluate.noVerdict', { fields: row.no_verdict.join(', ') })}
                    </p>
                    <p className="text-foreground-muted type-caption">{t('evaluate.noVerdictHint')}</p>
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>
    </Dialog>
  );
}
