'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { PeriodFilterBar, type Granularity } from '@/components/domain/period-filter-bar';
import type { ExportFormat } from '@/components/domain/export-menu';
import { FilterBar } from '@/components/shell/filter-bar';
import { PageHeader } from '@/components/shell/page-header';
import { EmptyState } from '@/components/ui/empty-state';
import { Tabs } from '@/components/ui/tabs';
import type { DateRange } from '@/components/ui/date-range-picker';
import { RefreshActions } from '@/features/jobs/refresh-actions';
import { ScopePicker } from '@/features/scope/scope-picker';
import { downloadFile } from '@/lib/api/download';
import { $api } from '@/lib/api/query';
import type { ConsumptionRow } from '@/lib/api/types';
import { addDays, istanbulToday } from '@/lib/dates';
import { useScopeParams, useSelection } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { AlarmCheckDialogView } from './alarm-check-dialog';
import { ConsumptionTableView } from './consumption-table';
import { DetailedGraphsView, type GroupBy } from './detailed-graphs';
import { SeriesChartView, type SeriesToggles } from './series-chart';
import { SummaryCardsView } from './summary-cards';

/** The consumption screen of 01 §7.3 (R197 defaults: daily, last six months). */
export function ConsumptionPage() {
  const t = useTranslations('consumption');
  const { can } = useSession();
  const scope = useScopeParams();
  const { buildingId, analyzerId } = useSelection();
  const today = istanbulToday();

  const [activeOnly, setActiveOnly] = useState(false);
  const [tab, setTab] = useState('consumption');
  const [show, setShow] = useState<SeriesToggles>({ active: true, inductive: true, capacitive: false });
  const [groupBy, setGroupBy] = useState<GroupBy>('daily');
  const [compare, setCompare] = useState(false);
  const [busyFormat, setBusyFormat] = useState<ExportFormat | null>(null);
  const [alarmRow, setAlarmRow] = useState<ConsumptionRow | null>(null);
  const [draft, setDraft] = useState<{ granularity: Granularity; range: DateRange }>({
    granularity: 'daily',
    range: { from: addDays(today, -182), to: today },
  });
  const [applied, setApplied] = useState(draft);

  const subject = analyzerId ? { analyzer_id: analyzerId } : buildingId ? { building_id: buildingId } : null;
  const enabled = subject !== null;
  const query = { ...scope, ...subject, granularity: applied.granularity, from: applied.range.from, to: applied.range.to };

  const series = $api.useQuery('get', '/api/v1/consumption', { params: { query } }, { enabled });
  const summary = $api.useQuery('get', '/api/v1/consumption/summary', { params: { query } }, { enabled });
  const grouped = $api.useQuery(
    'get',
    '/api/v1/consumption/grouped',
    {
      params: {
        query: {
          ...scope,
          ...subject,
          from: applied.range.from,
          to: applied.range.to,
          group_by: groupBy,
          ...(compare ? { compare: 'previous' as const } : {}),
        },
      },
    },
    { enabled: enabled && tab === 'detailed' },
  );
  const anomaly = $api.useMutation('post', '/api/v1/anomaly/check');

  const rows = series.data?.items ?? [];
  const exportFile = async (format: ExportFormat) => {
    setBusyFormat(format);
    try {
      await downloadFile(
        '/api/v1/consumption/export',
        { ...query, format: format === 'excel' ? 'xlsx' : 'csv' } as Record<string, string | undefined>,
        `tuketim-${applied.range.from}-${applied.range.to}.${format === 'excel' ? 'xlsx' : 'csv'}`,
      );
    } finally {
      setBusyFormat(null);
    }
  };

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />
      <FilterBar>
        <ScopePicker activeOnly={activeOnly} onActiveOnlyChange={setActiveOnly} />
        <PeriodFilterBar
          granularity={draft.granularity}
          onGranularityChange={(granularity) => setDraft((d) => ({ ...d, granularity }))}
          range={draft.range}
          onRangeChange={(range) => setDraft((d) => ({ ...d, range }))}
          onApply={() => setApplied(draft)}
          today={today}
          applying={series.isFetching}
        />
        <RefreshActions analyzerId={analyzerId ?? null} />
      </FilterBar>

      {!enabled ? (
        <EmptyState title={t('selectAnalyzer')} description={t('chart.emptyHint')} />
      ) : (
        <>
          <SummaryCardsView summary={summary.data ?? null} loading={summary.isFetching} />
          <Tabs
            value={tab}
            onValueChange={setTab}
            items={[
              {
                value: 'consumption',
                label: t('tabs.consumption'),
                content: (
                  <div className="flex flex-col gap-4">
                    <SeriesChartView rows={rows} granularity={applied.granularity} show={show} onShowChange={setShow} loading={series.isFetching} />
                    <ConsumptionTableView
                      rows={rows}
                      granularity={applied.granularity}
                      canCheckAlarm={can('anomaly.check') && Boolean(analyzerId)}
                      onCheckAlarm={(row) => {
                        setAlarmRow(row);
                        anomaly.mutate({
                          params: { query: scope },
                          body: { analyzer_id: analyzerId ?? '', ts: row.period_start, actual: row.active_import ?? '0' },
                        });
                      }}
                      onExport={exportFile}
                      onPrint={() => window.print()}
                      busyFormat={busyFormat}
                      loading={series.isFetching}
                    />
                  </div>
                ),
              },
              {
                value: 'detailed',
                label: t('tabs.detailed'),
                content: (
                  <DetailedGraphsView
                    data={grouped.data ?? null}
                    groupBy={groupBy}
                    onGroupByChange={setGroupBy}
                    compare={compare}
                    onCompareChange={setCompare}
                    show={show}
                    onShowChange={setShow}
                    loading={grouped.isFetching}
                  />
                ),
              },
            ]}
          />
        </>
      )}

      <AlarmCheckDialogView
        open={alarmRow !== null}
        onOpenChange={(open) => !open && setAlarmRow(null)}
        row={alarmRow}
        granularity={applied.granularity}
        result={anomaly.data ?? null}
        loading={anomaly.isPending}
      />
    </div>
  );
}
