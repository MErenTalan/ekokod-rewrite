'use client';

import { FileSpreadsheet, FileText } from 'lucide-react';
import { useLocale, useTranslations } from 'next-intl';

import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { EmptyState } from '@/components/ui/empty-state';
import type { Option } from '@/components/ui/field';
import { Select } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { StatusBadge, type Status } from '@/components/ui/status-badge';
import type { Locale } from '@/i18n/locale';
import type { ReportSummary } from '@/lib/api/types';
import { formatMonth } from '@/lib/format';

export type ArchiveFilters = { type: 'all' | 'monthly' | 'yearly'; buildingId: string };

export type ArchiveGroup = { year: string; yearly?: ReportSummary; monthly: ReportSummary[] };

/** R275: year → the yearly report and that year's monthly reports, newest first. */
export function groupArchive(items: ReportSummary[]): ArchiveGroup[] {
  const byYear = new Map<string, ArchiveGroup>();
  for (const r of items) {
    const year = r.period.slice(0, 4);
    const group = byYear.get(year) ?? { year, monthly: [] };
    if (r.type === 'yearly') group.yearly = r;
    else group.monthly.push(r);
    byYear.set(year, group);
  }
  const groups = [...byYear.values()].sort((a, b) => b.year.localeCompare(a.year));
  for (const g of groups) g.monthly.sort((a, b) => b.period.localeCompare(a.period));
  return groups;
}

const TONE: Record<ReportSummary['status'], Status> = { completed: 'success', pending: 'info', error: 'danger' };

export type ArchiveTabViewProps = {
  items: ReportSummary[];
  total: number;
  loading: boolean;
  filters: ArchiveFilters;
  buildings: Option[];
  onFilters: (f: ArchiveFilters) => void;
  onDownload: (reportId: string, format: 'pdf' | 'excel') => void;
  /** The API has more pages than those shown (R275). */
  hasMore?: boolean;
  loadingMore?: boolean;
  onLoadMore?: () => void;
};

/** The reports archive of 01 §7.14. */
export function ArchiveTabView({ items, total, loading, filters, buildings, onFilters, onDownload, hasMore = false, loadingMore = false, onLoadMore }: ArchiveTabViewProps) {
  const t = useTranslations('reports.archive');
  const actions = useTranslations('reports.actions');
  const locale = useLocale() as Locale;

  const entry = (r: ReportSummary) => {
    const title = r.type === 'yearly' ? `${t('yearlyReport')} ${r.period}` : formatMonth(r.period, locale);
    const name = `${title} · ${r.building_name}`;
    return (
      <li key={r.id} aria-label={name} className="flex flex-wrap items-center justify-between gap-3 border-b border-border py-2 last:border-b-0">
        <div className="flex flex-col">
          <span className="type-body font-semibold">{title}</span>
          <span className="text-foreground-muted type-small">{r.building_name}</span>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <StatusBadge status={TONE[r.status]} label={t(`status.${r.status}`)} />
          {r.status === 'completed' ? (
            <>
              <Button variant="secondary" size="sm" iconStart={FileText} onClick={() => onDownload(r.id, 'pdf')}>{actions('pdf')}</Button>
              <Button variant="secondary" size="sm" iconStart={FileSpreadsheet} onClick={() => onDownload(r.id, 'excel')}>{actions('excel')}</Button>
            </>
          ) : null}
        </div>
      </li>
    );
  };

  return (
    <div className="flex flex-col gap-6 pt-4">
      <Card className="grid gap-4 md:grid-cols-3 md:items-end">
        <Select
          label={t('filterType')}
          options={[
            { value: 'all', label: t('allTypes') },
            { value: 'monthly', label: t('monthly') },
            { value: 'yearly', label: t('yearly') },
          ]}
          value={filters.type}
          onValueChange={(v) => onFilters({ ...filters, type: v as ArchiveFilters['type'] })}
        />
        <Select
          label={t('filterBuilding')}
          options={[{ value: 'all', label: t('allBuildings') }, ...buildings]}
          value={filters.buildingId}
          onValueChange={(buildingId) => onFilters({ ...filters, buildingId })}
        />
        <p className="type-body font-semibold" aria-live="polite">{t('total', { count: total })}</p>
      </Card>

      {loading ? (
        <Skeleton className="h-64 w-full" aria-label={t('loading')} />
      ) : items.length === 0 ? (
        <EmptyState title={t('empty')} description={t('emptyDescription')} />
      ) : (
        groupArchive(items).map((g) => (
          <Card key={g.year} className="flex flex-col gap-4">
            <h2 className="type-h3">{t('yearTitle', { year: g.year })}</h2>
            <section className="flex flex-col gap-1">
              <h3 className="type-small font-semibold text-foreground-muted">{t('yearlyReport')}</h3>
              {g.yearly ? <ul>{entry(g.yearly)}</ul> : <p className="text-foreground-muted type-small">{t('noYearly')}</p>}
            </section>
            {g.monthly.length ? (
              <section className="flex flex-col gap-1">
                <h3 className="flex items-center gap-2 type-small font-semibold text-foreground-muted">
                  {t('monthlyReports')}
                  <span className="rounded-full border border-border px-2 type-caption">{t('monthCount', { count: g.monthly.length })}</span>
                </h3>
                <ul>{g.monthly.map(entry)}</ul>
              </section>
            ) : null}
          </Card>
        ))
      )}
      {hasMore && onLoadMore ? (
        <Button variant="secondary" className="self-center" loading={loadingMore} onClick={onLoadMore}>
          {t('loadMore')}
        </Button>
      ) : null}
    </div>
  );
}
