'use client';

import { Download } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { IconButton } from '@/components/ui/icon-button';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Analyzer, Bill } from '@/lib/api/types';
import { formatNumber } from '@/lib/format';

export type BillHistoryViewProps = {
  bills: Bill[];
  buildingNames: Record<string, string>;
  analyzers: Pick<Analyzer, 'id' | 'installation_number' | 'customer_name'>[];
  loading?: boolean;
  /** More bills exist than were fetched; older months are reached with the month picker. */
  truncated?: boolean;
  onOpenMonth: (period: string) => void;
  onDownloadPdf: (billID: string) => void;
};

/** Every computed bill in scope, newest first, like legacy's month-less invoice list. */
export function BillHistoryView({
  bills, buildingNames, analyzers, loading = false, truncated = false, onOpenMonth, onDownloadPdf,
}: BillHistoryViewProps) {
  const t = useTranslations('bills');
  const meter = new Map(analyzers.map((a) => [a.id, a.customer_name ? `${a.installation_number} · ${a.customer_name}` : a.installation_number]));
  const money = (v: string) => formatNumber(v, { minFractionDigits: 2, maxFractionDigits: 2 });
  const num = (v: string) => formatNumber(v, { maxFractionDigits: 2 });

  return (
    <section aria-labelledby="bill-history" className="flex flex-col gap-3">
      <div>
        <h3 id="bill-history" className="type-h3">{t('history.title')}</h3>
        <p className="text-foreground-muted type-caption">{t('history.description')}</p>
      </div>
      {loading ? (
        <Skeleton className="h-32 w-full" />
      ) : bills.length === 0 ? (
        <EmptyState title={t('history.empty')} description={t('history.emptyDescription')} />
      ) : (
        <TableContainer label={t('history.tableLabel')}>
          <Table aria-label={t('history.tableLabel')}>
            <TableHeader>
              <TableRow>
                <TableHead>{t('columns.period')}</TableHead>
                <TableHead>{t('history.kind')}</TableHead>
                <TableHead>{t('columns.building')}</TableHead>
                <TableHead>{t('columns.analyzer')}</TableHead>
                <TableHead>{t('columns.consumption')}</TableHead>
                <TableHead>{t('columns.invoice')}</TableHead>
                <TableHead>{t('columns.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {bills.map((b) => (
                <TableRow key={b.id}>
                  <TableCell>
                    <Button size="sm" variant="ghost" onClick={() => onOpenMonth(b.period_key)}
                      aria-label={t('history.open', { period: b.period_key })}>
                      {b.period_key}
                    </Button>
                  </TableCell>
                  <TableCell>{t(`history.scope.${b.scope}`)}</TableCell>
                  <TableCell>{b.building_id ? (buildingNames[b.building_id] ?? '—') : '—'}</TableCell>
                  <TableCell>{b.analyzer_id ? (meter.get(b.analyzer_id) ?? '—') : '—'}</TableCell>
                  <TableCell className="tabular-nums">{num(b.active_import)}</TableCell>
                  <TableCell className="whitespace-nowrap tabular-nums">{money(b.total_cost)} {b.currency}</TableCell>
                  <TableCell>
                    <IconButton label={t('history.download', { period: b.period_key })} icon={Download}
                      onClick={() => onDownloadPdf(b.id)} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
      {truncated ? <p className="text-foreground-muted type-caption">{t('history.truncated')}</p> : null}
    </section>
  );
}
