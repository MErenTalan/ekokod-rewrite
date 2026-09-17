'use client';

import { useTranslations } from 'next-intl';

import { formatCurrency, formatNumber, unitSymbol, type Unit } from '@/lib/format';

import { Badge } from '../ui/badge';
import { Table, TableBody, TableCaption, TableCell, TableContainer, TableFooter, TableHead, TableHeader, TableRow } from '../ui/table';

export type InvoiceLineView = {
  id: string;
  description: string;
  quantity: string | null;
  unit: Unit | null;
  unitPrice: string | null;
  amount: string;
  kind: 'energy' | 'distribution' | 'tax' | 'penalty' | 'other';
};
export type InvoiceLineTableProps = { lines: InvoiceLineView[]; currency: 'TRY'; totals: { label: string; amount: string }[]; caption: string };

// Unit prices keep their stated precision (tariffs quote ₺2,1840), never fewer than two digits.
const pricePrecision = (v: string | null) => {
  const digits = Math.max(2, (v?.split('.')[1] ?? '').length);
  return { minFractionDigits: digits, maxFractionDigits: digits };
};

/** Charge breakdown shared by the bill page and the PDF (07 §6); F8 maps F4's bill DTO to InvoiceLineView (plan D17). */
export function InvoiceLineTable({ lines, totals, caption }: InvoiceLineTableProps) {
  const t = useTranslations('domain.invoice');
  return (
    <TableContainer label={caption}>
      <Table>
        <TableCaption>{caption}</TableCaption>
        <TableHeader>
          <TableRow>
            <TableHead>{t('description')}</TableHead>
            <TableHead numeric>{t('quantity')}</TableHead>
            <TableHead numeric>{t('unitPrice')}</TableHead>
            <TableHead numeric>{t('amount')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {lines.map((line) => (
            <TableRow key={line.id}>
              <TableCell>
                <span className="flex flex-wrap items-center gap-2">
                  {line.description}
                  {line.kind === 'penalty' ? <Badge tone="danger">{t('penalty')}</Badge> : null}
                </span>
              </TableCell>
              <TableCell numeric>{line.quantity === null ? '—' : `${formatNumber(line.quantity)}${line.unit ? ` ${unitSymbol(line.unit)}` : ''}`}</TableCell>
              <TableCell numeric>{formatCurrency(line.unitPrice, pricePrecision(line.unitPrice))}</TableCell>
              <TableCell numeric>{formatCurrency(line.amount)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
        {totals.length > 0 ? (
          <TableFooter>
            {totals.map((total) => (
              <TableRow key={total.label}>
                <TableHead scope="row" colSpan={3} className="text-foreground type-body">
                  {total.label}
                </TableHead>
                <TableCell numeric>{formatCurrency(total.amount)}</TableCell>
              </TableRow>
            ))}
          </TableFooter>
        ) : null}
      </Table>
    </TableContainer>
  );
}
