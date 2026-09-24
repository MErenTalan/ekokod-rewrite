'use client';

import { useTranslations } from 'next-intl';

import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { EmptyState } from '@/components/ui/empty-state';
import { Table, TableBody, TableCaption, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { formatNumber } from '@/lib/format';
import type { BillDashboard } from '@/lib/api/types';

export type PlantsSectionViewProps = {
  plants: BillDashboard['plants'];
  onDownloadPdf?: (billID: string) => void;
};

/**
 * The plant section of 01 §7.10 (R290): one row per company plant. Analyzer
 * columns belong to the optional netting analyzer; missing figures say so.
 * A building-scoped reader never sees plants, so nothing renders for them.
 */
export function PlantsSectionView({ plants, onDownloadPdf }: PlantsSectionViewProps) {
  const t = useTranslations('bills');
  if (!plants.available) return null;

  const num = (v?: string) => (v ? formatNumber(v, { maxFractionDigits: 2 }) : t('plants.noData'));
  const money = (v: string) => formatNumber(v, { minFractionDigits: 2, maxFractionDigits: 2 });

  return (
    <Card className="flex flex-col gap-4">
      <div>
        <h3 className="type-h3">{t('plants.title')}</h3>
        <p className="text-foreground-muted type-caption">{t('plants.description')}</p>
      </div>
      {plants.rows.length === 0 ? (
        <EmptyState title={t('plants.empty')} description={t('plants.emptyDescription')} />
      ) : (
        <TableContainer label={t('plants.title')}>
          <Table>
            <TableCaption>{t('plants.title')}</TableCaption>
            <TableHeader>
              <TableRow>
                <TableHead>{t('plants.columns.plant')}</TableHead>
                <TableHead>{t('plants.columns.analyzer')}</TableHead>
                <TableHead>{t('plants.columns.installationNumber')}</TableHead>
                <TableHead>{t('plants.columns.production')}</TableHead>
                <TableHead>{t('plants.columns.consumptionPrice')}</TableHead>
                <TableHead>{t('plants.columns.productionPrice')}</TableHead>
                <TableHead>{t('plants.columns.sale')}</TableHead>
                <TableHead>{t('plants.columns.currency')}</TableHead>
                <TableHead>{t('columns.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {plants.rows.map((r) => (
                <TableRow key={r.plant_id}>
                  <TableCell>{r.plant_name}</TableCell>
                  <TableCell>{r.analyzer_name ?? '—'}</TableCell>
                  <TableCell>{r.installation_number ?? '—'}</TableCell>
                  <TableCell className="type-data">{num(r.production_kwh)}</TableCell>
                  <TableCell className="type-data">{num(r.consumption_price)}</TableCell>
                  <TableCell className="type-data">{num(r.production_price)}</TableCell>
                  <TableCell className="type-data">{r.invoice_amount ? money(r.invoice_amount) : t('plants.noData')}</TableCell>
                  <TableCell>{r.currency ?? '—'}</TableCell>
                  <TableCell>
                    {r.bill_id && onDownloadPdf ? (
                      <Button variant="ghost" size="sm" onClick={() => onDownloadPdf(r.bill_id as string)}>
                        {t('plants.download')}
                      </Button>
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
      <dl className="flex flex-wrap gap-x-6 gap-y-1 type-small">
        <div className="flex gap-2">
          <dt className="text-foreground-muted">{t('plants.totalProduction')}</dt>
          <dd className="type-data">{num(plants.total_production_kwh)} kWh</dd>
        </div>
        {plants.total_invoice.map((m) => (
          <div key={m.currency} className="flex gap-2">
            <dt className="text-foreground-muted">{t('plants.totalSale')}</dt>
            <dd className="type-data">{money(m.amount)} {m.currency}</dd>
          </div>
        ))}
      </dl>
    </Card>
  );
}
