'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { PageHeader } from '@/components/shell/page-header';
import { ScopePicker } from '@/features/scope/scope-picker';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { MonthPicker } from '@/components/ui/month-picker';
import { downloadFile } from '@/lib/api/download';
import { useApiMutation, useFailureToast } from '@/lib/api/mutation';
import { fieldErrors } from '@/lib/api/problem';
import type { BillPreview, Translator } from '@/lib/api/types';
import { $api } from '@/lib/api/query';
import { useScopeParams, useSelection } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { useJob } from '../jobs/use-job';
import { CustomPeriodCardView, type PreviewRequest, type SaveRequest } from './custom-period-card';
import { DashboardTableView } from './dashboard-table';
import { GenerateCardView, type GenerateRequest } from './generate-card';
import { messageKeyForErrorCode } from './generation-status';
import { NettingSummaryView } from './netting-summary';
import { PlantsSectionView } from './plants-section';

const ANALYZER_LIMIT = 500;

/** The bills screen of 01 §7.10: the month's dashboard and ad-hoc generation. */
export function BillsPage() {
  const t = useTranslations('bills');
  const { can } = useSession();
  const scope = useScopeParams();
  const { buildingId } = useSelection();

  const [month, setMonth] = useState<string | null>(null);
  const [activeOnly, setActiveOnly] = useState(false);
  const [jobID, setJobID] = useState<string | null>(null);
  // What the last generation asked for, so the finished invoice can be found
  // and offered for download (R238: compute -> watch -> download).
  const [lastRequest, setLastRequest] = useState<GenerateRequest | null>(null);

  const canCompute = can('bills.compute');
  const [year, monthNumber] = month ? month.split('-').map(Number) : [0, 0];

  const dashboard = $api.useQuery(
    'get',
    '/api/v1/bills/dashboard',
    { params: { query: { ...scope, year, month: monthNumber } } },
    { enabled: month !== null },
  );

  const analyzers = $api.useQuery(
    'get',
    '/api/v1/analyzers',
    { params: { query: { ...scope, limit: ANALYZER_LIMIT } } },
    { enabled: canCompute },
  );

  const compute = useApiMutation('post', '/api/v1/bills/compute', { invalidate: ['/api/v1/bills'] });
  const { job, errorCode } = useJob(jobID, t('generate.title'));

  const generate = (request: GenerateRequest) => {
    setLastRequest(request);
    compute.mutate(
      {
        params: { query: scope },
        body: {
          scope: request.scope,
          period: request.period,
          ...(request.scope === 'analyzer' ? { analyzer_ids: request.analyzerIds } : {}),
          ...(request.scope === 'building' && request.buildingId ? { building_ids: [request.buildingId] } : {}),
          force: true,
        },
      },
      { onSuccess: (data) => setJobID((data as { job_ids?: string[] }).job_ids?.[0] ?? null) },
    );
  };

  // F30: preview any window (a GET, so not the save-convention mutation) and save it as a period.
  const forms = useTranslations('forms') as unknown as Translator;
  const failureToast = useFailureToast();
  const [preview, setPreview] = useState<BillPreview | undefined>(undefined);
  const [previewErrors, setPreviewErrors] = useState<Record<string, string>>({});
  const previewBill = $api.useMutation('get', '/api/v1/bills/preview');
  const runPreview = (r: PreviewRequest) => {
    setPreviewErrors({});
    previewBill.mutate(
      { params: { query: { ...scope, scope: r.scope, subject_id: r.subjectId, from: r.from, to: r.to } } },
      {
        onSuccess: (data) => setPreview(data),
        onError: (error) => {
          setPreview(undefined);
          const fields = fieldErrors(error, forms);
          setPreviewErrors(fields);
          if (Object.keys(fields).length === 0) failureToast(error);
        },
      },
    );
  };
  const periods = $api.useQuery(
    'get',
    '/api/v1/buildings/{id}/bill-periods',
    { params: { path: { id: buildingId ?? '' }, query: scope } },
    { enabled: canCompute && !!buildingId },
  );
  const savePeriod = useApiMutation('put', '/api/v1/buildings/{id}/bill-periods/{period}', {
    success: t('custom.saved'), invalidate: ['/api/v1/buildings', '/api/v1/bills'],
  });
  const deletePeriod = useApiMutation('delete', '/api/v1/buildings/{id}/bill-periods/{period}', {
    success: t('custom.deleted'), invalidate: ['/api/v1/buildings', '/api/v1/bills'],
  });
  const runSave = (r: SaveRequest) => {
    if (!buildingId) return;
    savePeriod.mutate({
      params: { path: { id: buildingId, period: r.periodKey }, query: scope }, body: { from: r.from, to: r.to },
    });
  };
  const runDelete = (key: string, done: () => void) => {
    if (!buildingId) return;
    deletePeriod.mutate({ params: { path: { id: buildingId, period: key }, query: scope } }, { onSuccess: done });
  };

  const download = (path: string, query: Record<string, string | undefined>, name: string) =>
    void downloadFile(path, { ...scope, ...query }, name);

  // The compute job answers with a state, not a bill, so the finished invoice
  // is looked up by exactly what was asked for.
  const generated = $api.useQuery(
    'get',
    '/api/v1/bills',
    {
      params: {
        query: {
          ...scope,
          period: lastRequest?.period ?? '',
          scope: lastRequest?.scope ?? 'company',
          ...(lastRequest?.scope === 'analyzer' && lastRequest.analyzerIds.length === 1
            ? { analyzer_id: lastRequest.analyzerIds[0] }
            : {}),
          ...(lastRequest?.scope === 'building' && lastRequest.buildingId
            ? { building_id: lastRequest.buildingId }
            : {}),
        },
      },
    },
    { enabled: lastRequest !== null && job?.status === 'succeeded' },
  );

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />

      <div className="w-full max-w-xs">
        <MonthPicker label={t('monthSelection')} value={month} onValueChange={setMonth} />
      </div>

      {/* The building invoice of §7.10 and the custom period (F30) need a
          building, so the screen carries the same picker the consumption screens do. */}
      {canCompute ? <ScopePicker activeOnly={activeOnly} onActiveOnlyChange={setActiveOnly} /> : null}

      {month === null ? (
        <EmptyState title={t('noMonth')} description={t('noMonthDescription')} />
      ) : (
        <>
          {canCompute ? (
            <GenerateCardView
              analyzers={analyzers.data?.items ?? []}
              period={month}
              buildingId={buildingId ?? null}
              onGenerate={generate}
              running={compute.isPending || job?.status === 'queued' || job?.status === 'running'}
              failure={job?.status === 'failed' ? messageKeyForErrorCode(errorCode) : undefined}
              readyBillIds={job?.status === 'succeeded' ? (generated.data?.items ?? []).map((b) => b.id) : []}
              missingDays={job?.status === 'succeeded'
                ? [...new Set((generated.data?.items ?? []).flatMap((b) => b.missing_days ?? []))].sort()
                : []}
              onDownload={(id) => download(`/api/v1/bills/${id}/pdf`, {}, `fatura-${id}.pdf`)}
            />
          ) : null}

          {dashboard.isError ? (
            <EmptyState
              title={t('noData')}
              description={t('noDataDescription')}
              action={<Button onClick={() => void dashboard.refetch()}>{t('retry')}</Button>}
            />
          ) : (
            <>
              <DashboardTableView
                buildings={dashboard.data?.buildings ?? []}
                loading={dashboard.isPending}
                onDownloadPdf={(id) => download(`/api/v1/bills/${id}/pdf`, {}, `fatura-${id}.pdf`)}
                onDownloadHourly={(id) =>
                  download(`/api/v1/bills/${id}/hourly-detail`, { format: 'xlsx' }, `saatlik-${id}.xlsx`)
                }
                onDownloadAll={() =>
                  download('/api/v1/bills/dashboard/export', { year: String(year), month: String(monthNumber) },
                    `fatura-panosu-${month}.xlsx`)
                }
                onDownloadAllPdf={() =>
                  download('/api/v1/bills/dashboard/export',
                    { year: String(year), month: String(monthNumber), format: 'pdf' }, `fatura-panosu-${month}.pdf`)
                }
              />
              {dashboard.data ? (
                <>
                  <NettingSummaryView netting={dashboard.data.netting} />
                  <PlantsSectionView plants={dashboard.data.plants}
                    onDownloadPdf={(id) => download(`/api/v1/bills/${id}/pdf`, {}, `fatura-${id}.pdf`)} />
                </>
              ) : null}
            </>
          )}
        </>
      )}

      {canCompute ? (
        <CustomPeriodCardView
          key={buildingId ?? 'none'}
          analyzers={analyzers.data?.items ?? []}
          buildingId={buildingId ?? null}
          savedPeriods={periods.data ?? []}
          preview={preview}
          errors={{ ...previewErrors, ...savePeriod.fieldErrors }}
          previewing={previewBill.isPending}
          saving={savePeriod.isPending || deletePeriod.isPending}
          onPreview={runPreview}
          onSave={runSave}
          onDelete={runDelete}
        />
      ) : null}
    </div>
  );
}
