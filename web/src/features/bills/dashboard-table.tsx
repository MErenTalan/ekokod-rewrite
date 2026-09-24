'use client';

import { useTranslations } from 'next-intl';

import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { formatNumber } from '@/lib/format';
import type { BillDashboardBuilding, BillDashboardRow } from '@/lib/api/types';

export type DashboardTableViewProps = {
  buildings: BillDashboardBuilding[];
  onDownloadPdf?: (billID: string) => void;
  onDownloadHourly?: (billID: string) => void;
  onDownloadAll?: () => void;
  onDownloadAllPdf?: () => void;
  loading?: boolean;
};

/**
 * The invoice table of 01 §7.10. §7.10 describes a "buildings section" and a
 * "bill listing table" separately, but their columns are the same set and
 * legacy renders exactly one table — so this is one table (R250).
 */
export function DashboardTableView({
  buildings, onDownloadPdf, onDownloadHourly, onDownloadAll, onDownloadAllPdf, loading = false,
}: DashboardTableViewProps) {
  const t = useTranslations('bills');

  if (loading) return <Skeleton className="h-64 w-full" />;
  if (buildings.length === 0) {
    return <EmptyState title={t('noData')} description={t('noDataDescription')} />;
  }

  // The currency is a column of its own (R253 keeps currencies apart), so the
  // amount is formatted as a number and never stamped with a ₺ it may not be.
  const money = (v: string) => formatNumber(v, { minFractionDigits: 2, maxFractionDigits: 2 });
  const num = (v: string) => formatNumber(v, { maxFractionDigits: 2 });
  const diverges = buildings.some((b) => b.diverges_from_rows);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="type-h3">{t('buildings.title')}</h3>
          <p className="text-foreground-muted type-caption">{t('buildings.description')}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {onDownloadAll ? <Button variant="secondary" onClick={onDownloadAll}>{t('downloadAll')}</Button> : null}
          {onDownloadAllPdf ? <Button variant="ghost" onClick={onDownloadAllPdf}>{t('downloadAllPdf')}</Button> : null}
        </div>
      </div>

      <TableContainer label={t('buildings.title')}>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('columns.period')}</TableHead>
              <TableHead>{t('columns.building')}</TableHead>
              <TableHead>{t('columns.analyzer')}</TableHead>
              <TableHead>{t('columns.installationNumber')}</TableHead>
              <TableHead>{t('columns.etso')}</TableHead>
              <TableHead>{t('columns.consumption')}</TableHead>
              <TableHead>{t('columns.consumptionPrice')}</TableHead>
              <TableHead>{t('columns.invoice')}</TableHead>
              <TableHead>{t('columns.actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {buildings.map((building) => (
              <BuildingSection
                key={building.building_id}
                building={building}
                money={money}
                num={num}
                onDownloadPdf={onDownloadPdf}
                onDownloadHourly={onDownloadHourly}
              />
            ))}
          </TableBody>
        </Table>
      </TableContainer>

      {/* 02 §6.11: the building tariff is applied once to the aggregate, so a
          building invoice may legitimately differ from the sum of the rows.
          It is shown and explained, never reconciled away (R234). */}
      {diverges ? (
        <p role="note" className="text-foreground-muted type-caption">{t('divergenceNote')}</p>
      ) : null}
    </div>
  );
}

function BuildingSection({
  building, money, num, onDownloadPdf, onDownloadHourly,
}: {
  building: BillDashboardBuilding;
  money: (v: string) => string;
  num: (v: string) => string;
  onDownloadPdf?: (billID: string) => void;
  onDownloadHourly?: (billID: string) => void;
}) {
  const t = useTranslations('bills');
  const row = (r: BillDashboardRow) => (
    <TableRow key={r.bill_id}>
      <TableCell>{r.period_key}</TableCell>
      <TableCell>{r.building_name}</TableCell>
      <TableCell>{r.analyzer_name}</TableCell>
      <TableCell>{r.installation_number}</TableCell>
      <TableCell>{r.etso_code}</TableCell>
      <TableCell className="type-data">{num(r.consumption)}</TableCell>
      <TableCell className="type-data">{r.consumption_price ? num(r.consumption_price) : '—'}</TableCell>
      <TableCell className="type-data">{money(r.invoice)}</TableCell>
      <TableCell>
        <div className="flex flex-wrap gap-2">
          {onDownloadPdf ? (
            <Button variant="ghost" size="sm" onClick={() => onDownloadPdf(r.bill_id)}>{t('downloadPdf')}</Button>
          ) : null}
          {onDownloadHourly ? (
            <Button variant="ghost" size="sm" onClick={() => onDownloadHourly(r.bill_id)}>{t('downloadHourly')}</Button>
          ) : null}
        </div>
      </TableCell>
    </TableRow>
  );

  return (
    <>
      {building.rows.map(row)}
      <TableRow className="font-medium">
        <TableCell>{t('subtotal')}</TableCell>
        <TableCell>{building.building_name}</TableCell>
        <TableCell />
        <TableCell />
        <TableCell />
        <TableCell className="type-data">{num(building.total_consumption)}</TableCell>
        <TableCell />
        <TableCell className="type-data">{money(building.total_invoice)}</TableCell>
        <TableCell />
      </TableRow>
      {building.building_bill ? (
        <TableRow>
          <TableCell>{t('buildingBill')}</TableCell>
          <TableCell>{building.building_name}</TableCell>
          <TableCell />
          <TableCell />
          <TableCell />
          <TableCell className="type-data">{num(building.building_bill.consumption)}</TableCell>
          <TableCell />
          <TableCell className="type-data">{money(building.building_bill.invoice)}</TableCell>
          <TableCell>
            {onDownloadPdf ? (
              <Button variant="ghost" size="sm" onClick={() => onDownloadPdf(building.building_bill!.bill_id)}>
                {t('downloadPdf')}
              </Button>
            ) : null}
          </TableCell>
        </TableRow>
      ) : null}
    </>
  );
}
