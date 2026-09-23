'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { FilterBar } from '@/components/shell/filter-bar';
import { PageHeader } from '@/components/shell/page-header';
import { EmptyState } from '@/components/ui/empty-state';
import { Select } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { $api } from '@/lib/api/query';
import { istanbulToday } from '@/lib/dates';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { HeadlineCards } from './headline-cards';
import { MonthlyTable } from './monthly-table';
import { OffsetCard } from './offset-card';
import { TariffPanel } from './tariff-panel';
import { YearlyChart } from './yearly-chart';

const WHOLE_YEAR = 'all';

/** The financial analysis screen of 01 §7.9: company-wide, from invoices and plant production (R294). */
export function FinancialPage() {
  const t = useTranslations('financial');
  const { can } = useSession();
  const scope = useScopeParams();
  const thisYear = Number(istanbulToday().slice(0, 4));
  const [year, setYear] = useState(thisYear);
  const [month, setMonth] = useState<string>(WHOLE_YEAR);
  const allowed = can('financial.read');

  const summaryQuery = {
    ...scope,
    year,
    ...(month === WHOLE_YEAR ? {} : { month: Number(month) }),
  };
  const summary = $api.useQuery(
    'get',
    '/api/v1/financial/summary',
    { params: { query: summaryQuery } },
    { enabled: allowed },
  );
  const monthly = $api.useQuery(
    'get',
    '/api/v1/financial/monthly',
    { params: { query: { ...scope, year } } },
    { enabled: allowed },
  );

  if (!allowed) {
    return (
      <div className="flex flex-col gap-6">
        <PageHeader title={t('title')} description={t('subtitle')} />
        <EmptyState title={t('noAccess')} description={t('noAccessHint')} />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />
      <FilterBar>
        <Select
          label={t('year')}
          value={String(year)}
          onValueChange={(v) => setYear(Number(v))}
          options={Array.from({ length: 5 }, (_, i) => String(thisYear - i)).map((y) => ({
            value: y,
            label: y,
          }))}
        />
        <Select
          label={t('month')}
          value={month}
          onValueChange={setMonth}
          options={[
            { value: WHOLE_YEAR, label: t('wholeYear') },
            ...Array.from({ length: 12 }, (_, i) => ({
              value: String(i + 1),
              label: t(`months.m${i + 1}` as 'months.m1'),
            })),
          ]}
        />
      </FilterBar>
      {summary.data ? (
        <>
          <HeadlineCards summary={summary.data} />
          <div className="grid gap-4 lg:grid-cols-2">
            <OffsetCard figures={summary.data.figures} />
            <TariffPanel tariffs={summary.data.tariffs} />
          </div>
        </>
      ) : (
        <Skeleton className="h-40 w-full" />
      )}
      <YearlyChart monthly={monthly.data} loading={monthly.isLoading} />
      {monthly.data ? <MonthlyTable monthly={monthly.data} /> : null}
    </div>
  );
}
