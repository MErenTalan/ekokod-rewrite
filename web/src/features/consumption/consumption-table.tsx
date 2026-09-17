'use client';

import type { ColumnDef } from '@tanstack/react-table';
import { Printer, Siren } from 'lucide-react';
import { useLocale, useTranslations } from 'next-intl';
import { useCallback, useMemo } from 'react';

import { DataQualityBadge, type DataQuality } from '@/components/domain/data-quality-badge';
import { ExportMenu, type ExportFormat } from '@/components/domain/export-menu';
import type { Granularity } from '@/components/domain/period-filter-bar';
import { Button } from '@/components/ui/button';
import { DataTable } from '@/components/ui/data-table';
import type { Locale } from '@/i18n/locale';
import type { ConsumptionRow } from '@/lib/api/types';
import { fractionToPercent } from '@/lib/decimal';
import { formatNumber, unitSymbol } from '@/lib/format';
import { formatPeriod } from '@/lib/format-period';

import { CONSUMPTION_COLUMNS, type ColumnLabelKey } from './columns';

export type ConsumptionTableViewProps = {
  rows: ConsumptionRow[];
  granularity: Granularity;
  canCheckAlarm: boolean;
  onCheckAlarm: (row: ConsumptionRow) => void;
  onExport: (format: ExportFormat) => void;
  onPrint: () => void;
  busyFormat?: ExportFormat | null;
  loading?: boolean;
};

const value = (row: ConsumptionRow, key: keyof ConsumptionRow) => {
  const raw = row[key];
  return typeof raw === 'string' ? raw : null;
};

/** Every column 01 §7.3 lists (R206), with the quality of each period visible. */
export function ConsumptionTableView({
  rows,
  granularity,
  canCheckAlarm,
  onCheckAlarm,
  onExport,
  onPrint,
  busyFormat = null,
  loading = false,
}: ConsumptionTableViewProps) {
  const t = useTranslations('consumption');
  const locale = useLocale() as Locale;

  const quality = useCallback(
    (row: ConsumptionRow): DataQuality | undefined => {
      if (row.suspect_registers.length > 0) {
        const names = row.suspect_registers.map((r) => t(`columns.${REGISTER_LABEL[r] ?? 'active'}` as const)).join(', ');
        return { state: 'suspect', reason: t('table.suspectReason', { registers: names }) };
      }
      if (row.partial) return { state: 'incomplete', reason: t('table.partialReason') };
      return undefined;
    },
    [t],
  );

  const columns = useMemo<ColumnDef<ConsumptionRow, unknown>[]>(
    () =>
      CONSUMPTION_COLUMNS.map((column) => {
        const header = column.unit ? `${t(`columns.${column.labelKey}`)} (${unitSymbol(column.unit)})` : t(`columns.${column.labelKey}`);
        if (column.key === 'period') {
          return {
            id: 'period',
            header,
            accessorFn: (row: ConsumptionRow) => row.period_start,
            enableHiding: false,
            cell: ({ row }) => (
              <span className="flex items-center gap-2">
                {formatPeriod(row.original.period_start, granularity, locale)}
                {quality(row.original) ? <DataQualityBadge quality={quality(row.original)!} /> : null}
              </span>
            ),
          } satisfies ColumnDef<ConsumptionRow, unknown>;
        }
        const key = column.key as keyof ConsumptionRow;
        return {
          id: String(key),
          header: column.kind === 'ratio' ? `${t(`columns.${column.labelKey}`)} (%)` : header,
          accessorFn: (row: ConsumptionRow) => Number(value(row, key) ?? 0),
          meta: { numeric: true },
          cell: ({ row }) => {
            const raw = value(row.original, key);
            if (raw === null) return '—';
            return column.kind === 'ratio' ? `%${formatNumber(fractionToPercent(raw), { maxFractionDigits: 2 })}` : formatNumber(raw);
          },
        } satisfies ColumnDef<ConsumptionRow, unknown>;
      }),
    [t, granularity, locale, quality],
  );

  return (
    <DataTable
      columns={columns}
      data={rows}
      caption={t('table.caption')}
      getRowId={(row) => row.period_start + (row.analyzer_id ?? '')}
      pageSize={20}
      enableColumnVisibility
      loading={loading}
      empty={{ title: t('chart.empty'), description: t('chart.emptyHint') }}
      rowActions={
        canCheckAlarm
          ? (row) => [{ type: 'item' as const, label: t('table.alarmCheck'), icon: Siren, onSelect: () => onCheckAlarm(row) }]
          : undefined
      }
      toolbar={
        <div className="flex flex-wrap items-center gap-2">
          <ExportMenu formats={['csv', 'excel']} onExport={onExport} busyFormat={busyFormat} />
          <Button variant="secondary" iconStart={Printer} onClick={onPrint} className="print:hidden">
            {t('table.print')}
          </Button>
        </div>
      }
    />
  );
}

/** The register names the API reports as suspect, as column labels. */
const REGISTER_LABEL: Record<string, ColumnLabelKey> = {
  active_import: 'active',
  reactive_inductive_import: 'inductive',
  reactive_capacitive_import: 'capacitive',
  t1_import: 't1',
  t2_import: 't2',
  t3_import: 't3',
  active_export: 'activeGeneration',
  reactive_inductive_export: 'inductiveGeneration',
  reactive_capacitive_export: 'capacitiveGeneration',
  t1_export: 'u1',
  t2_export: 'u2',
  t3_export: 'u3',
};
