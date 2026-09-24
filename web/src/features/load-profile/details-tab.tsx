'use client';

import type { ColumnDef } from '@tanstack/react-table';
import { useTranslations } from 'next-intl';
import { useMemo } from 'react';

import { ExportMenu, type ExportFormat } from '@/components/domain/export-menu';
import { DataTable } from '@/components/ui/data-table';
import type { LoadProfileStatistics, ProfileStatistics, Translator } from '@/lib/api/types';
import { formatHour } from '@/lib/dates';
import { fractionToPercent } from '@/lib/decimal';
import { formatNumber } from '@/lib/format';

import { PROFILE_KEYS, profileLabel, type ProfileKey } from './profile-label';

type Row = { key: ProfileKey; label: string; stats: ProfileStatistics };

export type DetailsTabViewProps = {
  statistics: LoadProfileStatistics | null;
  onExport: (format: ExportFormat) => void;
  busyFormat?: ExportFormat | null;
  loading?: boolean;
};

/** The Details tab of 01 §7.4: per-profile statistics and the hourly matrix export. */
export function DetailsTabView({ statistics, onExport, busyFormat = null, loading = false }: DetailsTabViewProps) {
  const t = useTranslations('loadProfile') as unknown as Translator;
  const details = useTranslations('loadProfile.details');

  const rows = useMemo<Row[]>(
    () =>
      PROFILE_KEYS.filter((key) => statistics?.statistics?.[key]).map((key) => ({
        key,
        label: profileLabel(t, key),
        stats: statistics!.statistics[key] as ProfileStatistics,
      })),
    [statistics, t],
  );

  const number = (value?: string | null) => (value == null ? '—' : formatNumber(value));
  const columns = useMemo<ColumnDef<Row, unknown>[]>(
    () => [
      { accessorKey: 'label', header: details('profile'), enableHiding: false },
      { id: 'max', header: details('max'), accessorFn: (r) => Number(r.stats.max ?? 0), meta: { numeric: true }, cell: ({ row }) => number(row.original.stats.max) },
      { id: 'min', header: details('min'), accessorFn: (r) => Number(r.stats.min ?? 0), meta: { numeric: true }, cell: ({ row }) => number(row.original.stats.min) },
      {
        id: 'hourOfMax',
        header: details('hourOfMax'),
        accessorFn: (r) => r.stats.hour_of_max ?? -1,
        meta: { numeric: true },
        cell: ({ row }) => (row.original.stats.hour_of_max == null ? '—' : formatHour(row.original.stats.hour_of_max)),
      },
      { id: 'mean', header: details('mean'), accessorFn: (r) => Number(r.stats.mean ?? 0), meta: { numeric: true }, cell: ({ row }) => number(row.original.stats.mean) },
      { id: 'stddev', header: details('stddev'), accessorFn: (r) => Number(r.stats.stddev ?? 0), meta: { numeric: true }, cell: ({ row }) => number(row.original.stats.stddev) },
      { id: 'range', header: details('range'), accessorFn: (r) => Number(r.stats.range ?? 0), meta: { numeric: true }, cell: ({ row }) => number(row.original.stats.range) },
      {
        id: 'loadFactor',
        header: `${details('loadFactor')} (%)`,
        accessorFn: (r) => Number(r.stats.load_factor ?? 0),
        meta: { numeric: true },
        cell: ({ row }) =>
          row.original.stats.load_factor == null ? '—' : `%${formatNumber(fractionToPercent(row.original.stats.load_factor), { maxFractionDigits: 2 })}`,
      },
    ],
    [details],
  );

  return (
    <DataTable
      columns={columns}
      data={rows}
      caption={details('caption')}
      getRowId={(row) => row.key}
      pageSize={false}
      loading={loading}
      empty={{ title: details('empty'), description: details('emptyHint') }}
      toolbar={<ExportMenu formats={['excel']} onExport={onExport} busyFormat={busyFormat} />}
    />
  );
}
