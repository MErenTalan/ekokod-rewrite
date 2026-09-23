'use client';

import { Download } from 'lucide-react';
import { useLocale, useTranslations } from 'next-intl';

import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { IconButton } from '@/components/ui/icon-button';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Locale } from '@/i18n/locale';
import type { CarbonReportSummary } from '@/lib/api/types';
import { formatDate, formatDateTime } from '@/lib/format';

export type ReportHistoryProps = {
  rows: CarbonReportSummary[];
  hasMore: boolean;
  loadingMore?: boolean;
  onLoadMore: () => void;
  onDownload: (r: CarbonReportSummary) => void;
};

/** R326: the building's reports, newest first, each with its PDF. */
export function ReportHistory({ rows, hasMore, loadingMore = false, onLoadMore, onDownload }: ReportHistoryProps) {
  const t = useTranslations('carbon');
  const locale = useLocale() as Locale;
  const period = (p: string) => p.split('/').map((d) => formatDate(d, locale)).join(' – ');
  return (
    <section aria-labelledby="carbon-report-history" className="flex flex-col gap-3">
      <h2 id="carbon-report-history" className="type-h3">
        {t('reporting.historyTitle')}
      </h2>
      {rows.length === 0 ? (
        <EmptyState title={t('reporting.noHistory')} description={t('reporting.noHistoryHint')} />
      ) : (
        <TableContainer label={t('reporting.historyLabel')}>
          <Table aria-label={t('reporting.historyLabel')}>
            <TableHeader>
              <TableRow>
                <TableHead>{t('reporting.name')}</TableHead>
                <TableHead>{t('reporting.created')}</TableHead>
                <TableHead>{t('reporting.period')}</TableHead>
                <TableHead>{t('reporting.typeColumn')}</TableHead>
                <TableHead>{t('reporting.download')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((r) => (
                <TableRow key={r.id}>
                  <TableCell>{r.name}</TableCell>
                  <TableCell className="whitespace-nowrap">{formatDateTime(r.created_at, locale)}</TableCell>
                  <TableCell className="whitespace-nowrap">{period(r.period)}</TableCell>
                  <TableCell>{t(`reporting.${r.report_type}`)}</TableCell>
                  <TableCell>
                    <IconButton label={t('reporting.downloadName', { name: r.name })} icon={Download} onClick={() => onDownload(r)} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
      {hasMore ? (
        <Button variant="secondary" className="self-center" loading={loadingMore} onClick={onLoadMore}>
          {t('reporting.loadMore')}
        </Button>
      ) : null}
    </section>
  );
}
