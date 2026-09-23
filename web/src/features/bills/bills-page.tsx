'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { PageHeader } from '@/components/shell/page-header';
import { ScopePicker } from '@/features/scope/scope-picker';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { MonthPicker } from '@/components/ui/month-picker';
import { downloadFile } from '@/lib/api/download';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import { useScopeParams, useSelection } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { useJob } from '../jobs/use-job';
import { DashboardTableView } from './dashboard-table';
import { GenerateCardView, type GenerateRequest } from './generate-card';
import { messageKeyForErrorCode } from './generation-status';
import { NettingSummaryView } from './netting-summary';

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

  const download = (path: string, query: Record<string, string | undefined>, name: string) =>
    void downloadFile(path, { ...scope, ...query }, name);

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />

      <MonthPicker label={t('monthSelection')} value={month} onValueChange={setMonth} />

      {month === null ? (
        <EmptyState title={t('noMonth')} description={t('noMonthDescription')} />
      ) : (
        <>
          {/* The building invoice of §7.10 needs a building, so the screen
              carries the same picker the consumption screens do. */}
          {canCompute ? <ScopePicker activeOnly={activeOnly} onActiveOnlyChange={setActiveOnly} /> : null}
          {canCompute ? (
            <GenerateCardView
              analyzers={analyzers.data?.items ?? []}
              period={month}
              buildingId={buildingId ?? null}
              onGenerate={generate}
              running={compute.isPending || job?.status === 'queued' || job?.status === 'running'}
              failure={job?.status === 'failed' ? messageKeyForErrorCode(errorCode) : undefined}
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
                <NettingSummaryView netting={dashboard.data.netting} plants={dashboard.data.plants} />
              ) : null}
            </>
          )}
        </>
      )}
    </div>
  );
}
