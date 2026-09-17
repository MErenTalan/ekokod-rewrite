'use client';

import { ChevronLeft, ChevronRight } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useId } from 'react';

import { formatNumber } from '@/lib/format';

import { Button } from './button';

export type PaginationProps = {
  page: number;
  pageCount: number;
  onPageChange: (page: number) => void;
  pageSize?: number;
  pageSizeOptions?: number[];
  onPageSizeChange?: (size: number) => void;
  totalItems?: number;
};

export function Pagination({ page, pageCount, onPageChange, pageSize, pageSizeOptions, onPageSizeChange, totalItems }: PaginationProps) {
  const t = useTranslations('feedback');
  const sizeId = useId();
  const count = Math.max(1, pageCount);
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 type-small">
      <div className="flex items-center gap-3 text-foreground-muted">
        {pageSize && pageSizeOptions && onPageSizeChange ? (
          <span className="flex items-center gap-2">
            <label htmlFor={sizeId}>{t('rowsPerPage')}</label>
            {/* Native select keeps Pagination independent of the form Select (plan D25). */}
            <select
              id={sizeId}
              value={pageSize}
              onChange={(e) => onPageSizeChange(Number(e.target.value))}
              className="h-8 rounded-md border border-border-control bg-surface px-2 text-foreground type-small pointer-coarse:min-h-11"
            >
              {pageSizeOptions.map((size) => (
                <option key={size} value={size}>
                  {size}
                </option>
              ))}
            </select>
          </span>
        ) : null}
        {totalItems !== undefined ? <span>{t('totalItems', { count: formatNumber(totalItems) })}</span> : null}
      </div>
      <nav aria-label={t('pageOf', { page, count })} className="flex items-center gap-2">
        <Button variant="secondary" size="sm" iconStart={ChevronLeft} disabled={page <= 1} onClick={() => onPageChange(page - 1)} aria-label={t('previousPage')}>
          <span className="sr-only sm:not-sr-only">{t('previousPage')}</span>
        </Button>
        <span aria-live="polite" className="min-w-24 text-center text-foreground">
          {t('pageOf', { page, count })}
        </span>
        <Button variant="secondary" size="sm" iconEnd={ChevronRight} disabled={page >= count} onClick={() => onPageChange(page + 1)} aria-label={t('nextPage')}>
          <span className="sr-only sm:not-sr-only">{t('nextPage')}</span>
        </Button>
      </nav>
    </div>
  );
}
