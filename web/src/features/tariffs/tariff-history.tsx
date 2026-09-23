'use client';

import { useLocale, useTranslations } from 'next-intl';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Locale } from '@/i18n/locale';
import { formatDate } from '@/lib/format';
import type { TariffSummaryItem } from '@/lib/api/types';

import { optionKey } from './tariff-labels';

export type TariffHistoryViewProps = {
  tariffs: TariffSummaryItem[];
  /** tariffs.edit (A/CA) gates create, edit and delete. */
  canEdit: boolean;
  onCreate?: () => void;
  onEdit?: (id: string) => void;
  onDelete?: (id: string) => void;
  loading?: boolean;
};

/**
 * The version list of 01 §7.11. A building holds a LIST of tariffs, each with
 * an effective date, and a bill is always priced with the version in force at
 * the time — which is why deleting one is spelled out rather than silent.
 */
export function TariffHistoryView({ tariffs, canEdit, onCreate, onEdit, onDelete, loading = false }: TariffHistoryViewProps) {
  const t = useTranslations('tariffs');
  const common = useTranslations('common');
  const locale = useLocale() as Locale;

  if (loading) return <Skeleton className="h-64 w-full" />;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <p className="text-foreground-muted type-caption">{t('historyNote')}</p>
        {canEdit && onCreate ? <Button onClick={onCreate}>{t('newTariff')}</Button> : null}
      </div>

      {tariffs.length === 0 ? (
        <EmptyState title={t('historyEmpty')} description={t('historyNote')} />
      ) : (
        <TableContainer label={t('history')}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('columns.effectiveFrom')}</TableHead>
                <TableHead>{t('columns.name')}</TableHead>
                <TableHead>{t('columns.priceType')}</TableHead>
                <TableHead>{t('columns.term')}</TableHead>
                <TableHead>{t('columns.pricing')}</TableHead>
                {canEdit ? <TableHead>{common('actions')}</TableHead> : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {tariffs.map((tariff) => (
                <TableRow key={tariff.id}>
                  <TableCell className="font-medium">{formatDate(tariff.effective_from, locale)}</TableCell>
                  <TableCell>{tariff.name ?? '—'}</TableCell>
                  <TableCell>{t(`options.priceType.${optionKey(tariff.price_type)}` as never)}</TableCell>
                  <TableCell>{t(`options.term.${optionKey(tariff.term)}` as never)}</TableCell>
                  <TableCell>
                    <Badge tone={tariff.use_ptf_yekdem ? 'info' : 'neutral'}>
                      {tariff.use_ptf_yekdem ? t('ptfBadge') : t('fixedBadge')}
                    </Badge>
                  </TableCell>
                  {canEdit ? (
                    <TableCell>
                      <div className="flex flex-wrap gap-2">
                        <Button variant="ghost" size="sm" onClick={() => onEdit?.(tariff.id)}>{common('edit')}</Button>
                        <Button variant="ghost" size="sm" onClick={() => onDelete?.(tariff.id)}>{common('delete')}</Button>
                      </div>
                    </TableCell>
                  ) : null}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </div>
  );
}
