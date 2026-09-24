'use client';

import { useTranslations } from 'next-intl';

import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import type { FinancialMonth, FinancialMonthly } from '@/lib/api/types';
import { formatNumber } from '@/lib/format';

import { MoneyLines } from './money-lines';

/** R294's monthly table and year row; a month without data says so rather than showing 0. */
export function MonthlyTable({ monthly }: { monthly: FinancialMonthly }) {
  const t = useTranslations('financial');
  const kwh = (v?: string | null) =>
    v ? (
      formatNumber(v, { maxFractionDigits: 2 })
    ) : (
      <span className="text-foreground-muted type-small">{t('noData')}</span>
    );
  const cells = (m: FinancialMonth) => (
    <>
      <TableCell numeric>{kwh(m.consumption_kwh)}</TableCell>
      <TableCell numeric>
        <MoneyLines list={m.cost} />
      </TableCell>
      <TableCell numeric>{kwh(m.production_kwh)}</TableCell>
      <TableCell numeric>
        <MoneyLines list={m.revenue} />
        {m.revenue_partial ? (
          <Badge tone="warning">{t('table.partialMark')}</Badge>
        ) : null}
      </TableCell>
      <TableCell numeric>{kwh(m.offset_kwh)}</TableCell>
      <TableCell numeric>
        <MoneyLines list={m.net} />
      </TableCell>
    </>
  );
  return (
    <TableContainer label={t('table.label')}>
      <Table aria-label={t('table.label')}>
        <TableHeader>
          <TableRow>
            <TableHead>{t('table.month')}</TableHead>
            {(['consumption', 'cost', 'production', 'revenue', 'offset', 'net'] as const).map(
              (k) => (
                <TableHead key={k} numeric>
                  {t(`table.${k}`)}
                </TableHead>
              ),
            )}
          </TableRow>
        </TableHeader>
        <TableBody>
          {monthly.items.map((m) => (
            <TableRow key={m.month}>
              <TableCell>{t(`months.m${m.month}` as 'months.m1')}</TableCell>
              {cells(m)}
            </TableRow>
          ))}
        </TableBody>
        <TableFooter>
          <TableRow>
            <TableCell className="font-semibold">{t('table.total')}</TableCell>
            {cells(monthly.total)}
          </TableRow>
        </TableFooter>
      </Table>
    </TableContainer>
  );
}
