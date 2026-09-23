'use client';

import { Check, Pencil, Trash2, X } from 'lucide-react';
import { useLocale, useTranslations } from 'next-intl';

import { FilterBar } from '@/components/shell/filter-bar';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { IconButton } from '@/components/ui/icon-button';
import { Select } from '@/components/ui/select';
import { StatusBadge } from '@/components/ui/status-badge';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Locale } from '@/i18n/locale';
import type { CarbonActivity } from '@/lib/api/types';
import { formatDate, formatNumber } from '@/lib/format';

import { isoKey, scopeKey, STATUS_TONE, subKey } from './labels';

export type StatusFilters = { status: 'all' | CarbonActivity['status']; scope: 'all' | CarbonActivity['scope'] };

export type StatusTableProps = {
  rows: CarbonActivity[];
  editable: boolean;
  filters: StatusFilters;
  onFilters: (f: StatusFilters) => void;
  onApprove: (a: CarbonActivity) => void;
  onReject: (a: CarbonActivity) => void;
  onEdit: (a: CarbonActivity) => void;
  onDelete: (a: CarbonActivity) => void;
  hasMore: boolean;
  loadingMore?: boolean;
  onLoadMore: () => void;
};

export const periodLabel = (a: CarbonActivity, locale: Locale) =>
  a.period_start === a.period_end
    ? formatDate(a.period_start, locale)
    : `${formatDate(a.period_start, locale)} – ${formatDate(a.period_end, locale)}`;

/** R325: every record with its approval status; A CA act on the row. */
export function StatusTable({ rows, editable, filters, onFilters, onApprove, onReject, onEdit, onDelete, hasMore, loadingMore = false, onLoadMore }: StatusTableProps) {
  const t = useTranslations('carbon');
  const locale = useLocale() as Locale;
  return (
    <div className="flex flex-col gap-4">
      <FilterBar>
        <Select
          label={t('statusTab.filterStatus')}
          value={filters.status}
          onValueChange={(v) => onFilters({ ...filters, status: v as StatusFilters['status'] })}
          options={[{ value: 'all', label: t('statusTab.allStatuses') }, ...(['pending', 'approved', 'rejected'] as const).map((s) => ({ value: s, label: t(`status.${s}`) }))]}
        />
        <Select
          label={t('statusTab.filterScope')}
          value={filters.scope}
          onValueChange={(v) => onFilters({ ...filters, scope: v as StatusFilters['scope'] })}
          options={[{ value: 'all', label: t('statusTab.allScopes') }, ...(['scope_1', 'scope_2', 'scope_3'] as const).map((s) => ({ value: s, label: t(scopeKey(s)) }))]}
        />
      </FilterBar>
      {rows.length === 0 ? (
        <EmptyState title={t('statusTab.empty')} description={t('statusTab.emptyHint')} />
      ) : (
        <TableContainer label={t('statusTab.label')}>
          <Table aria-label={t('statusTab.label')}>
            <TableHeader>
              <TableRow>
                <TableHead>{t('statusTab.period')}</TableHead>
                <TableHead>{t('statusTab.activity')}</TableHead>
                <TableHead numeric>{t('statusTab.quantity')}</TableHead>
                <TableHead numeric>{t('statusTab.emission')}</TableHead>
                <TableHead>{t('statusTab.scope')}</TableHead>
                <TableHead>{t('statusTab.iso')}</TableHead>
                <TableHead>{t('statusTab.status')}</TableHead>
                {editable ? <TableHead>{t('statusTab.actions')}</TableHead> : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((a) => (
                <TableRow key={a.id}>
                  <TableCell className="whitespace-nowrap">{periodLabel(a, locale)}</TableCell>
                  <TableCell>
                    {t(subKey(a.sub_category))}
                    {a.is_automated ? <span className="ms-2 text-foreground-muted type-caption">{t('statusTab.automated')}</span> : null}
                  </TableCell>
                  <TableCell numeric className="whitespace-nowrap">{`${formatNumber(a.quantity)} ${a.unit}`}</TableCell>
                  <TableCell numeric>{formatNumber(a.emission_kgco2e, { minFractionDigits: 2, maxFractionDigits: 2 })}</TableCell>
                  <TableCell>{t(scopeKey(a.scope))}</TableCell>
                  <TableCell>{t(isoKey(a.iso_category))}</TableCell>
                  <TableCell>
                    <StatusBadge status={STATUS_TONE[a.status]} label={t(`status.${a.status}`)} />
                  </TableCell>
                  {editable ? (
                    <TableCell>
                      <div className="flex gap-1">
                        {a.status !== 'approved' ? <IconButton label={t('statusTab.approve')} icon={Check} onClick={() => onApprove(a)} /> : null}
                        {a.status !== 'rejected' ? <IconButton label={t('statusTab.reject')} icon={X} onClick={() => onReject(a)} /> : null}
                        {!a.is_automated ? (
                          <>
                            <IconButton label={t('statusTab.edit')} icon={Pencil} onClick={() => onEdit(a)} />
                            <IconButton label={t('statusTab.delete')} icon={Trash2} onClick={() => onDelete(a)} />
                          </>
                        ) : null}
                      </div>
                    </TableCell>
                  ) : null}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
      {hasMore ? (
        <Button variant="secondary" className="self-center" loading={loadingMore} onClick={onLoadMore}>
          {t('statusTab.loadMore')}
        </Button>
      ) : null}
    </div>
  );
}
