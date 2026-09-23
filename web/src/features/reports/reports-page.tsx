'use client';

import { useLocale, useTranslations } from 'next-intl';
import { useEffect, useMemo, useState } from 'react';

import { PageHeader } from '@/components/shell/page-header';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs } from '@/components/ui/tabs';
import { $api } from '@/lib/api/query';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { useApiMutation } from '@/lib/api/mutation';
import { downloadFile } from '@/lib/api/download';
import type { ReportGenerateItem } from '@/lib/api/types';
import { useToast } from '@/components/ui/toast';
import { formatMonth } from '@/lib/format';
import type { Locale } from '@/i18n/locale';

import { useJob } from '../jobs/use-job';
import { ArchiveTabView, type ArchiveFilters } from './archive-tab';
import { EmailDialog } from './email-dialog';
import { MonthlyReportView } from './monthly-report';
import { ReportActionsView } from './report-actions';
import { useReportJobs } from './use-report-jobs';
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
  const actions = useReportActions(kind, selection, buildingOptions);
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
      {actions}
    </div>
  );
}

/**
 * E-3: generate one report per selected building, watch each job, then offer
 * that building's files and e-mail. A changed selection clears the rows, so a
 * download never belongs to a selection no longer on screen.
 */
function useReportActions(kind: ReportKind, selection: ReportSelection, buildings: { value: string; label: string }[]) {
  const t = useTranslations('reports');
  const { can } = useSession();
  const scope = useScopeParams();
  const locale = useLocale() as Locale;
  const toast = useToast();
  const [items, setItems] = useState<ReportGenerateItem[]>([]);
  const [emailFor, setEmailFor] = useState<string | null>(null);
  const [emailJob, setEmailJob] = useState<string | null>(null);
  const generate = useApiMutation('post', '/api/v1/reports/generate', { invalidate: ['/api/v1/reports'] });
  const email = useApiMutation('post', '/api/v1/reports/{id}/email');
  const jobs = useReportJobs(items.map((i) => i.job_id));
  const { job: delivery, errorCode: deliveryCode } = useJob(emailJob, t('email.title'));
  const key = JSON.stringify(selection);

  useEffect(() => setItems([]), [key]);
  useEffect(() => {
    if (!emailJob || !delivery) return;
    if (delivery.status === 'succeeded') {
      toast.toast({ tone: 'success', title: t('email.sent') });
      setEmailJob(null);
    } else if (delivery.status === 'failed') {
      const message: Record<string, string> = {
        smtp_not_configured: t('email.errors.smtpNotConfigured'),
        report_not_ready: t('email.errors.reportNotReady'),
        delivery_failed: t('email.errors.deliveryFailed'),
      };
      toast.toast({ tone: 'danger', title: (deliveryCode && message[deliveryCode]) || t('email.errors.generic') });
      setEmailJob(null);
    }
  }, [delivery, deliveryCode, emailJob, t, toast]);

  if (!can('reports.read') || selection.period === null || selection.buildingIds.length === 0) return null;
  const name = (id: string) => buildings.find((b) => b.value === id)?.label ?? id;
  const period = selection.period;
  const periodLabel = kind === 'monthly' ? formatMonth(period, locale) : period;
  const target = items.find((i) => i.report_id === emailFor);

  return (
    <>
      <ReportActionsView
        canGenerate={can('reports.generate')}
        canEmail={can('reports.email')}
        generating={generate.isPending}
        rows={items.map((i) => ({ buildingId: i.building_id, buildingName: name(i.building_id), reportId: i.report_id, ...jobs[i.job_id] }))}
        onGenerate={() =>
          generate.mutate(
            {
              params: { query: scope },
              body: {
                type: kind, period, plant_selection: selection.plantSelection, building_ids: selection.buildingIds,
                plant_ids: selection.plantIds.length ? selection.plantIds : undefined,
              },
            },
            { onSuccess: (data) => setItems(data.items) },
          )
        }
        onDownload={(id, format) => void downloadFile(`/api/v1/reports/${id}/${format}`, scope, `rapor-${period}.${format === 'pdf' ? 'pdf' : 'xlsx'}`)}
        onEmail={setEmailFor}
      />
      <EmailDialog
        open={emailFor !== null}
        summary={{ period: periodLabel, building: target ? name(target.building_id) : '' }}
        sending={email.isPending}
        error={email.fieldErrors.to}
        onClose={() => setEmailFor(null)}
        onSend={(to) =>
          emailFor &&
          email.mutate(
            { params: { path: { id: emailFor }, query: scope }, body: { to: [to] } },
            {
              onSuccess: (data) => {
                toast.toast({ tone: 'info', title: t('email.queued') });
                setEmailJob(data.job_id);
                setEmailFor(null);
              },
            },
          )
        }
      />
    </>
  );
}

function ArchiveTab() {
  const scope = useScopeParams();
  const [filters, setFilters] = useState<ArchiveFilters>({ type: 'all', buildingId: 'all' });
  const buildings = $api.useQuery('get', '/api/v1/buildings', { params: { query: { ...scope, limit: 500 } } });
  // R275: the archive is paged; every page stays reachable through the cursor.
  const list = $api.useInfiniteQuery(
    'get',
    '/api/v1/reports',
    {
      params: {
        query: {
          ...scope,
          limit: 200,
          type: filters.type === 'all' ? undefined : filters.type,
          building_id: filters.buildingId === 'all' ? undefined : filters.buildingId,
        },
      },
    },
    {
      pageParamName: 'cursor',
      initialPageParam: undefined,
      getNextPageParam: (last: { next_cursor?: string | null }) => last.next_cursor ?? undefined,
    },
  );
  const pages = list.data?.pages ?? [];
  return (
    <ArchiveTabView
      items={pages.flatMap((p) => p.items)}
      total={pages[0]?.total ?? 0}
      loading={list.isPending}
      hasMore={list.hasNextPage}
      loadingMore={list.isFetchingNextPage}
      onLoadMore={() => void list.fetchNextPage()}
      filters={filters}
      buildings={(buildings.data?.items ?? []).map((b) => ({ value: b.id, label: b.name }))}
      onFilters={setFilters}
      onDownload={(id, format) => void downloadFile(`/api/v1/reports/${id}/${format}`, scope, `rapor.${format === 'pdf' ? 'pdf' : 'xlsx'}`)}
    />
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
          { value: 'archive', label: t('tabs.archive'), content: <ArchiveTab /> },
        ]}
      />
    </div>
  );
}
