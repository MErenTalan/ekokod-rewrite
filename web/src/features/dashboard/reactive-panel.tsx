'use client';

import type { ColumnDef } from '@tanstack/react-table';
import Link from 'next/link';
import { useTranslations } from 'next-intl';
import { useEffect, useMemo, useState } from 'react';

import { ReactiveStatusCard } from '@/components/domain/reactive-status-card';
import { Button } from '@/components/ui/button';
import { DataTable } from '@/components/ui/data-table';
import { Dialog } from '@/components/ui/dialog';
import { MonthPicker } from '@/components/ui/month-picker';
import { Select } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { StatusBadge } from '@/components/ui/status-badge';
import { $api } from '@/lib/api/query';
import { istanbulToday, monthOf } from '@/lib/dates';
import { fractionToPercent } from '@/lib/decimal';
import { formatQuantity } from '@/lib/format';
import type { ReactiveAnalyzer } from '@/lib/api/types';
import { useScopeParams, useSelection } from '@/lib/selection/selection-store';

export type ReactiveRow = ReactiveAnalyzer & { analyzerLabel: string; buildingName: string };

export type ReactivePanelViewProps = {
  month: string;
  maxMonth: string;
  onMonthChange: (month: string) => void;
  scope: string;
  scopeOptions: { value: string; label: string }[];
  onScopeChange: (value: string) => void;
  rows: ReactiveRow[];
  highestInductive?: ReactiveRow;
  highestCapacitive?: ReactiveRow;
  loading?: boolean;
};

// The API's exempt reasons (R166) and the line each one shows.
const EXEMPT = {
  below_kw: 'exemptBelowKw',
  term: 'exemptMonomial',
  user_group: 'exemptUserGroup',
  generation: 'exemptGeneration',
} as const;

/**
 * The monthly reactive-penalty card of 01 §7.2: the worst inductive and
 * capacitive analyzers against their own limits (R166), an advisory line, and
 * every analyzer in a sortable dialog with a link to its invoice.
 */
export function ReactivePanelView({
  month,
  maxMonth,
  onMonthChange,
  scope,
  scopeOptions,
  onScopeChange,
  rows,
  highestInductive,
  highestCapacitive,
  loading = false,
}: ReactivePanelViewProps) {
  const t = useTranslations('dashboard.reactive');
  const [open, setOpen] = useState(false);
  const penalty = rows.some((r) => r.penalty_applies);

  const columns = useMemo<ColumnDef<ReactiveRow, unknown>[]>(
    () => [
      { accessorKey: 'buildingName', header: t('building'), enableHiding: false },
      { accessorKey: 'analyzerLabel', header: t('analyzer') },
      {
        id: 'inductive',
        header: t('inductive'),
        accessorFn: (r) => Number(r.inductive_ratio ?? -1),
        meta: { numeric: true },
        cell: ({ row }) => percent(row.original.inductive_ratio),
      },
      {
        id: 'capacitive',
        header: t('capacitive'),
        accessorFn: (r) => Number(r.capacitive_ratio ?? -1),
        meta: { numeric: true },
        cell: ({ row }) => percent(row.original.capacitive_ratio),
      },
      {
        id: 'penalty',
        header: t('penalty'),
        accessorFn: (r) => (r.penalty_applies ? 1 : 0),
        cell: ({ row }) =>
          row.original.exempt_reason ? (
            <span className="text-foreground-muted type-small">{t(EXEMPT[row.original.exempt_reason])}</span>
          ) : (
            <StatusBadge
              status={row.original.penalty_applies ? 'danger' : 'success'}
              label={row.original.penalty_applies ? t('penaltyYes') : t('penaltyNo')}
            />
          ),
      },
      {
        id: 'bill',
        header: t('viewBill'),
        cell: ({ row }) => (
          <Button asChild size="sm" variant="ghost">
            <Link href={`/ekorm/bills?building_id=${row.original.building_id ?? ''}&period=${month}`}>{t('viewBill')}</Link>
          </Button>
        ),
      },
    ],
    [t, month],
  );

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-end gap-3">
        <div className="w-40">
          <MonthPicker label={t('month')} value={month} onValueChange={(v) => onMonthChange(v ?? month)} max={maxMonth} />
        </div>
        <div className="w-48">
          <Select label={t('scope')} options={scopeOptions} value={scope} onValueChange={onScopeChange} />
        </div>
      </div>
      {loading ? (
        <Skeleton className="h-64 w-full" />
      ) : (
        <>
          <ReactiveStatusCard
            period={month}
            inductive={{ ratio: highestInductive?.inductive_ratio ?? null, limit: highestInductive?.inductive_limit ?? '0.20' }}
            capacitive={{ ratio: highestCapacitive?.capacitive_ratio ?? null, limit: highestCapacitive?.capacitive_limit ?? '0.15' }}
            penaltyApplied={rows.length === 0 ? null : penalty}
            advisory={rows.length === 0 ? t('noData') : penalty ? t('advisoryFix') : t('advisoryOk')}
          />
          <div className="flex flex-col gap-1 text-foreground-muted type-small">
            {highestInductive ? (
              <p>{t('highestInductive', { analyzer: highestInductive.analyzerLabel, building: highestInductive.buildingName })}</p>
            ) : null}
            {highestCapacitive ? (
              <p>{t('highestCapacitive', { analyzer: highestCapacitive.analyzerLabel, building: highestCapacitive.buildingName })}</p>
            ) : null}
          </div>
          <Button variant="secondary" size="sm" className="self-start" onClick={() => setOpen(true)} disabled={rows.length === 0}>
            {t('openDialog')}
          </Button>
          <Dialog open={open} onOpenChange={setOpen} title={t('dialogTitle')} size="lg">
            <DataTable
              columns={columns}
              data={rows}
              caption={t('dialogTitle')}
              getRowId={(row) => row.analyzer_id}
              initialSorting={[{ id: 'inductive', desc: true }]}
              empty={{ title: t('noData'), description: t('noData') }}
            />
          </Dialog>
        </>
      )}
    </div>
  );
}

const percent = (ratio: string | null | undefined) =>
  ratio == null ? '—' : formatQuantity(fractionToPercent(ratio), 'percent', { maxFractionDigits: 2 });

/** Feeds the view from `/consumption/reactive-status` for the chosen month and scope (R166). */
export function ReactivePanel({
  buildings,
  analyzerNames,
}: {
  buildings: { id: string; name: string }[];
  analyzerNames: Record<string, string>;
}) {
  const t = useTranslations('dashboard.reactive');
  const scopeParams = useScopeParams();
  const { buildingId } = useSelection();
  const [month, setMonth] = useState(() => monthOf(istanbulToday()));
  const [scope, setScope] = useState('all');

  const buildingFilter = scope === 'all' ? undefined : scope;
  const query = $api.useQuery('get', '/api/v1/consumption/reactive-status', {
    params: { query: { ...scopeParams, month, ...(buildingFilter ? { building_id: buildingFilter } : {}) } },
  });

  const rows = useMemo<ReactiveRow[]>(
    () =>
      (query.data?.analyzers ?? []).map((a) => ({
        ...a,
        analyzerLabel: analyzerNames[a.analyzer_id] ?? a.analyzer_id,
        buildingName: buildings.find((b) => b.id === a.building_id)?.name ?? t('unnamedBuilding'),
      })),
    [query.data, analyzerNames, buildings, t],
  );
  const find = (id?: string) => rows.find((r) => r.analyzer_id === id);

  useEffect(() => {
    // Selecting a building on the dashboard narrows this card too (01 §7.2).
    setScope(buildingId ?? 'all');
  }, [buildingId]);

  return (
    <ReactivePanelView
      month={month}
      maxMonth={monthOf(istanbulToday())}
      onMonthChange={setMonth}
      scope={scope}
      scopeOptions={[{ value: 'all', label: t('allBuildings') }, ...buildings.map((b) => ({ value: b.id, label: b.name }))]}
      onScopeChange={setScope}
      rows={rows}
      highestInductive={find(query.data?.highest_inductive?.analyzer_id)}
      highestCapacitive={find(query.data?.highest_capacitive?.analyzer_id)}
      loading={query.isLoading}
    />
  );
}
