'use client';

import { useLocale, useTranslations } from 'next-intl';

import type { Granularity } from '@/components/domain/period-filter-bar';
import { Alert } from '@/components/ui/alert';
import { Dialog } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import type { Locale } from '@/i18n/locale';
import type { AnomalyCheck, ConsumptionRow } from '@/lib/api/types';
import { formatQuantity } from '@/lib/format';
import { formatPeriod } from '@/lib/format-period';

export type AlarmCheckDialogViewProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  row: ConsumptionRow | null;
  granularity: Granularity;
  result: AnomalyCheck | null;
  loading?: boolean;
};

/**
 * The row action of 01 §7.3. Until F13 the model answers `available:false`
 * (R165) and this says so — it never invents a verdict (10 item 12).
 */
export function AlarmCheckDialogView({
  open,
  onOpenChange,
  row,
  granularity,
  result,
  loading = false,
}: AlarmCheckDialogViewProps) {
  const t = useTranslations('consumption.alarm');
  const locale = useLocale() as Locale;
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={t('title')}>
      {row ? (
        <dl className="flex flex-col gap-2">
          <div className="flex items-baseline justify-between gap-2">
            <dt className="text-foreground-muted type-caption">{t('date')}</dt>
            <dd className="type-body">{formatPeriod(row.period_start, granularity, locale)}</dd>
          </div>
          <div className="flex items-baseline justify-between gap-2">
            <dt className="text-foreground-muted type-caption">{t('actual')}</dt>
            <dd className="type-data">{formatQuantity(row.active_import ?? null, 'kWh')}</dd>
          </div>
        </dl>
      ) : null}
      {loading ? <Skeleton className="mt-3 h-12 w-full" /> : null}
      {!loading && result && !result.available ? <Alert className="mt-3" tone="info" title={t('unavailable')} /> : null}
    </Dialog>
  );
}
