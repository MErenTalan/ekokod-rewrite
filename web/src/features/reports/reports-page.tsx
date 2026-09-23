'use client';

import { useTranslations } from 'next-intl';
import { useEffect, useMemo, useState } from 'react';

import { PageHeader } from '@/components/shell/page-header';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs } from '@/components/ui/tabs';
import { $api } from '@/lib/api/query';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { MonthlyReportView } from './monthly-report';
import { YearlyReportView } from './yearly-report';
import { ReportSelectionView, type PlantOption, type ReportKind, type ReportSelection } from './report-selection';
import { useReportPreview } from './use-report-preview';

/** The previous calendar month and year, which are the ones a report closes on (R268). */
export function defaultPeriod(kind: ReportKind, now = new Date()): string {
  if (kind === 'yearly') return String(now.getFullYear() - 1);
  const d = new Date(now.getFullYear(), now.getMonth() - 1, 1);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
}

function ReportTab({ kind }: { kind: ReportKind }) {
  const t = useTranslations('reports');
  const { can } = useSession();
  const scope = useScopeParams();
  const buildings = $api.useQuery('get', '/api/v1/buildings', { params: { query: { ...scope, limit: 500 } } });
  const canPlants = can('nav.solar_plants');
  const plants = $api.useQuery('get', '/api/v1/power-plants', { params: { query: scope } }, { enabled: canPlants });

  const buildingOptions = useMemo(() => (buildings.data?.items ?? []).map((b) => ({ value: b.id, label: b.name })), [buildings.data]);
  const plantOptions: PlantOption[] | null = canPlants
    ? (plants.data?.items ?? []).map((p) => ({ value: p.id, label: p.name, kind: p.plant_kind }))
    : null;
  const years = useMemo(() => Array.from({ length: 5 }, (_, i) => new Date().getFullYear() - i), []);

  const [selection, setSelection] = useState<ReportSelection>({
    buildingIds: [], period: defaultPeriod(kind), plantSelection: 'all', plantIds: [],
  });
  // The first building is the default, once the list has arrived.
  useEffect(() => {
    if (selection.buildingIds.length === 0 && buildingOptions.length > 0) {
      setSelection((s) => ({ ...s, buildingIds: [buildingOptions[0].value] }));
    }
  }, [buildingOptions, selection.buildingIds.length]);

  const preview = useReportPreview(kind, selection);
  const monthly = preview.data?.monthly;
  const yearly = preview.data?.yearly;

  let body;
  if (selection.buildingIds.length === 0 || selection.period === null) {
    body = <EmptyState title={t('noSelection')} description={t('noSelectionDescription')} />;
  } else if (preview.isError) {
    body = (
      <EmptyState
        title={t('loadFailed')}
        description={t('loadFailedDescription')}
        action={<Button onClick={() => void preview.refetch()}>{t('retry')}</Button>}
      />
    );
  } else if (preview.isPending) {
    body = <Skeleton className="h-96 w-full" />;
  } else if (kind === 'monthly' && monthly) {
    body = <MonthlyReportView payload={monthly} />;
  } else if (kind === 'yearly' && yearly) {
    body = <YearlyReportView payload={yearly} />;
  }

  return (
    <div className="flex flex-col gap-6 pt-4">
      <ReportSelectionView kind={kind} buildings={buildingOptions} plants={plantOptions} value={selection} onChange={setSelection} years={years} />
      {body}
    </div>
  );
}

/** The reports screen of 01 §7.14. */
export function ReportsPage() {
  const t = useTranslations('reports');
  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />
      <Tabs
        defaultValue="monthly"
        items={[
          { value: 'monthly', label: t('tabs.monthly'), content: <ReportTab kind="monthly" /> },
          { value: 'yearly', label: t('tabs.yearly'), content: <ReportTab kind="yearly" /> },
        ]}
      />
    </div>
  );
}
