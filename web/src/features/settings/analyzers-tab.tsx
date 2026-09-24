'use client';

import type { ColumnDef } from '@tanstack/react-table';
import { useTranslations } from 'next-intl';
import { useMemo } from 'react';

import type { JobView } from '@/components/domain/job-status-banner';
import { JobStatusBanner } from '@/components/domain/job-status-banner';
import { DataTable } from '@/components/ui/data-table';
import { SearchInput } from '@/components/ui/search-input';
import { Select } from '@/components/ui/select';
import { StatusBadge } from '@/components/ui/status-badge';
import type { Analyzer, Building, Provider } from '@/lib/api/types';
import { formatDate, formatNumber } from '@/lib/format';
import type { Locale } from '@/i18n/locale';
import { useLocale } from 'next-intl';

export type AnalyzerFilters = { q: string; buildingId: string; provider: string };

export const ALL = 'all';
export const UNASSIGNED = 'unassigned';

export type AnalyzersTabViewProps = {
  analyzers: Analyzer[];
  buildings: Building[];
  providers: Provider[];
  filters: AnalyzerFilters;
  onFiltersChange: (filters: AnalyzerFilters) => void;
  /** Assigning a building is A/CA only (R191); refreshing is A/CA/BA (R164). */
  canEdit: boolean;
  canRefresh: boolean;
  onAssign: (analyzer: Analyzer) => void;
  onRefresh: (analyzer: Analyzer, mode: 'hourly' | 'energy') => void;
  job?: JobView | null;
  onDismissJob?: () => void;
  loading?: boolean;
};

/** The metering points of 01 §7.15 with their filters and per-row actions. */
export function AnalyzersTabView({
  analyzers,
  buildings,
  providers,
  filters,
  onFiltersChange,
  canEdit,
  canRefresh,
  onAssign,
  onRefresh,
  job = null,
  onDismissJob,
  loading = false,
}: AnalyzersTabViewProps) {
  const t = useTranslations('settings.analyzers');
  const refresh = useTranslations('domain.refresh');
  const locale = useLocale() as Locale;

  const columns = useMemo<ColumnDef<Analyzer, unknown>[]>(
    () => [
      { accessorKey: 'installation_number', header: t('installationNumber'), enableHiding: false },
      { id: 'customer', header: t('customerName'), accessorFn: (row) => row.customer_name ?? '—' },
      { id: 'meter', header: t('meterNumber'), accessorFn: (row) => row.meter_number ?? '—' },
      { id: 'model', header: t('meterModel'), accessorFn: (row) => row.meter_model ?? '—' },
      {
        id: 'multiplier',
        header: t('multiplier'),
        accessorFn: (row) => Number(row.meter_multiplier),
        meta: { numeric: true },
        cell: ({ row }) => formatNumber(row.original.meter_multiplier),
      },
      { id: 'province', header: t('province'), accessorFn: (row) => [row.province, row.district].filter(Boolean).join(' / ') || '—' },
      { id: 'tariff', header: t('tariffType'), accessorFn: (row) => row.tariff_type ?? '—' },
      {
        id: 'power',
        header: t('installedPower'),
        accessorFn: (row) => Number(row.installed_power_kw ?? 0),
        meta: { numeric: true },
        cell: ({ row }) => formatNumber(row.original.installed_power_kw ?? null),
      },
      {
        id: 'building',
        header: t('building'),
        accessorFn: (row) => buildings.find((b) => b.id === row.building_id)?.name ?? '—',
      },
      {
        id: 'lastData',
        header: t('lastData'),
        accessorFn: (row) => row.last_reading_at ?? '',
        cell: ({ row }) => (row.original.last_reading_at ? formatDate(row.original.last_reading_at.slice(0, 10), locale) : '—'),
      },
      {
        id: 'status',
        header: t('status'),
        accessorFn: (row) => (row.activity_status === 'active' ? 1 : 0),
        cell: ({ row }) => (
          <StatusBadge
            status={row.original.activity_status === 'active' ? 'success' : 'neutral'}
            label={row.original.activity_status === 'active' ? t('active') : t('passive')}
          />
        ),
      },
    ],
    [t, buildings, locale],
  );

  const rowActions = (analyzer: Analyzer) => [
    ...(canEdit ? [{ type: 'item' as const, label: t('assign'), onSelect: () => onAssign(analyzer) }] : []),
    ...(canRefresh
      ? [
          { type: 'item' as const, label: refresh('hourly'), onSelect: () => onRefresh(analyzer, 'hourly') },
          { type: 'item' as const, label: refresh('energy'), onSelect: () => onRefresh(analyzer, 'energy') },
        ]
      : []),
  ];

  return (
    <div className="flex flex-col gap-4">
      <JobStatusBanner job={job} onDismiss={onDismissJob} />
      <div className="flex flex-wrap items-end gap-3">
        <div className="w-64 max-w-full">
          <SearchInput
            label={t('search')}
            value={filters.q}
            onValueChange={(q) => onFiltersChange({ ...filters, q })}
            placeholder={t('search')}
          />
        </div>
        <div className="w-56">
          <Select
            label={t('filterBuilding')}
            options={[
              { value: ALL, label: t('allBuildings') },
              { value: UNASSIGNED, label: t('unassigned') },
              ...buildings.map((b) => ({ value: b.id, label: b.name })),
            ]}
            value={filters.buildingId}
            onValueChange={(buildingId) => onFiltersChange({ ...filters, buildingId })}
          />
        </div>
        <div className="w-48">
          <Select
            label={t('filterProvider')}
            options={[{ value: ALL, label: t('allProviders') }, ...providers.map((p) => ({ value: p, label: p }))]}
            value={filters.provider}
            onValueChange={(provider) => onFiltersChange({ ...filters, provider })}
          />
        </div>
      </div>
      <DataTable
        columns={columns}
        data={analyzers}
        caption={t('title')}
        getRowId={(row) => row.id}
        loading={loading}
        empty={{ title: t('empty'), description: t('emptyHint') }}
        rowActions={rowActions(analyzers[0] ?? ({} as Analyzer)).length > 0 ? rowActions : undefined}
      />
    </div>
  );
}
