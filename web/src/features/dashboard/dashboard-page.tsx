'use client';

import { useTranslations } from 'next-intl';
import { useEffect, useMemo, useState } from 'react';

import { RefreshActions } from '@/features/jobs/refresh-actions';
import { analyzerLabel, useAssets } from '@/features/scope/use-assets';
import { PageHeader } from '@/components/shell/page-header';
import type { Granularity } from '@/components/domain/period-filter-bar';
import type { DateRange } from '@/components/ui/date-range-picker';
import { errorCodeOf } from '@/lib/api/problem';
import { $api } from '@/lib/api/query';
import type { Translator } from '@/lib/api/types';
import { istanbulToday } from '@/lib/dates';
import { toCsv } from '@/lib/csv';
import { useScopeParams, useSelection } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { ConsumptionPanelView, type SeriesToggles } from './consumption-panel';
import { BuildingListView } from './building-list';
import { LatestBillCard } from './latest-bill-card';
import { MapPanelView } from './map-panel';
import { ReactivePanel } from './reactive-panel';
import { sectorCsvRows, SectorComparisonView } from './sector-comparison';

const startOfYear = (today: string) => `${today.slice(0, 4)}-01-01`;

/** Saves a client-built CSV (the sectoral table) with the D22 conventions. */
function saveCsv(name: string, rows: string[][]) {
  const blob = new Blob([toCsv(rows)], { type: 'text/csv;charset=utf-8' });
  const href = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = href;
  anchor.download = name;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(href);
}

/** The dashboard of 01 §7.2, assembled from its panels (R204). */
export function DashboardPage() {
  const t = useTranslations('dashboard');
  const sector = useTranslations('dashboard.sector') as unknown as Translator;
  const { can } = useSession();
  const scope = useScopeParams();
  const { buildings, analyzers, loading } = useAssets();
  const { buildingId, analyzerId, set } = useSelection();
  const today = istanbulToday();

  const [visibleIds, setVisibleIds] = useState<string[] | null>(null);
  const [focusId, setFocusId] = useState<string | undefined>();
  const [show, setShow] = useState<SeriesToggles>({ active: true, inductive: true, capacitive: false });
  const [applied, setApplied] = useState<{ granularity: Granularity; range: DateRange }>({
    granularity: 'monthly',
    range: { from: startOfYear(today), to: today },
  });

  useEffect(() => {
    // Every building starts visible on the map, as the legacy list did.
    if (visibleIds === null && buildings.length > 0) setVisibleIds(buildings.map((b) => b.id));
  }, [buildings, visibleIds]);

  const shownIds = visibleIds ?? buildings.map((b) => b.id);
  const analyzerNames = useMemo(
    () => Object.fromEntries(analyzers.map((a) => [a.id, analyzerLabel(a)])),
    [analyzers],
  );
  const selectedBuilding = buildings.find((b) => b.id === buildingId);

  const subject = buildingId ? { building_id: buildingId } : {};
  const enabled = Boolean(buildingId);
  const seriesQuery = $api.useQuery(
    'get',
    '/api/v1/consumption',
    { params: { query: { ...scope, ...subject, granularity: applied.granularity, from: applied.range.from, to: applied.range.to } } },
    { enabled },
  );
  const sameYear = applied.range.from.slice(0, 4) === applied.range.to.slice(0, 4);
  const yoyEnabled = enabled && applied.granularity === 'monthly' && sameYear;
  const previousYear = $api.useQuery(
    'get',
    '/api/v1/consumption',
    {
      params: {
        query: {
          ...scope,
          ...subject,
          granularity: 'monthly',
          from: shiftYear(applied.range.from),
          to: shiftYear(applied.range.to),
        },
      },
    },
    { enabled: yoyEnabled },
  );

  const comparison = $api.useQuery(
    'get',
    '/api/v1/buildings/{id}/comparison',
    { params: { path: { id: buildingId ?? '' }, query: scope } },
    { enabled, retry: false, meta: { quietErrors: ['building_sector_missing'] } },
  );

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />
      {/* Spans sit on the grid's own children: StaggerGrid wraps each child and would swallow them. */}
      <div className="grid gap-4 lg:grid-cols-2 2xl:grid-cols-12">
        <div className="stagger-item min-w-0 lg:col-span-2 2xl:col-span-6" style={{ ['--i' as string]: 0 }}>
          <MapPanelView
            loading={loading}
            buildings={buildings
              .filter((b) => shownIds.includes(b.id))
              .map((b) => ({ id: b.id, name: b.name, lat: b.latitude ?? null, lng: b.longitude ?? null, status: b.activity_status ?? 'passive' }))}
            analyzers={analyzers.map((a) => ({
              id: a.id,
              name: analyzerLabel(a),
              lat: a.latitude ?? null,
              lng: a.longitude ?? null,
              status: a.activity_status,
            }))}
            focusId={focusId}
            onSelectBuilding={(id) => set({ buildingId: id })}
            onSelectAnalyzer={(id) => {
              const analyzer = analyzers.find((a) => a.id === id);
              set({ buildingId: analyzer?.building_id ?? buildingId, analyzerId: id });
            }}
          />
        </div>
        <div className="stagger-item min-w-0 2xl:col-span-3" style={{ ['--i' as string]: 1 }}>
          <BuildingListView
            loading={loading}
            buildings={buildings.map((b) => ({
              id: b.id,
              name: b.name,
              address: b.address ?? null,
              analyzerCount: b.analyzer_count ?? 0,
              status: b.activity_status ?? 'passive',
            }))}
            analyzers={analyzers.map((a) => ({ id: a.id, buildingId: a.building_id ?? null, name: analyzerLabel(a) }))}
            selectedBuildingId={buildingId}
            selectedAnalyzerId={analyzerId}
            visibleIds={shownIds}
            onVisibleChange={setVisibleIds}
            onSelectBuilding={(id) => set({ buildingId: id })}
            onSelectAnalyzer={(id) => set({ analyzerId: id })}
            onFocus={setFocusId}
          />
        </div>
        <div className="stagger-item flex min-w-0 flex-col gap-4 2xl:col-span-3" style={{ ['--i' as string]: 2 }}>
          <LatestBillCard buildingName={selectedBuilding?.name} />
          <ReactivePanel
            buildings={buildings.map((b) => ({ id: b.id, name: b.name }))}
            analyzerNames={analyzerNames}
          />
        </div>
      </div>
      <ConsumptionPanelView
        rows={seriesQuery.data?.items ?? []}
        previousYear={yoyEnabled ? (previousYear.data?.items ?? []) : null}
        granularity={applied.granularity}
        range={applied.range}
        today={today}
        onApply={(granularity, range) => setApplied({ granularity, range })}
        show={show}
        onShowChange={setShow}
        loading={seriesQuery.isFetching}
        actions={<RefreshActions analyzerId={analyzerId ?? null} />}
      />
      <SectorComparisonView
        buildingName={selectedBuilding?.name ?? null}
        comparison={comparison.data ?? null}
        sectorMissing={errorCodeOf(comparison.error) === 'building_sector_missing'}
        canEditBuildings={can('settings.buildings') && can('write')}
        loading={comparison.isLoading && enabled}
        onExport={() => {
          if (!comparison.data || !selectedBuilding) return;
          saveCsv(
            `${selectedBuilding.name}-sektor-karsilastirma.csv`,
            sectorCsvRows(comparison.data, selectedBuilding.name, sector),
          );
        }}
      />
    </div>
  );
}

const shiftYear = (iso: string) => `${Number(iso.slice(0, 4)) - 1}${iso.slice(4)}`;
