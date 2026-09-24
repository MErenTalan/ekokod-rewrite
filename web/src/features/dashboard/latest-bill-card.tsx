'use client';

import { Receipt } from 'lucide-react';
import Link from 'next/link';
import { useLocale, useTranslations } from 'next-intl';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { EmptyState } from '@/components/ui/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import { StatusBadge, type Status } from '@/components/ui/status-badge';
import { $api } from '@/lib/api/query';
import type { Bill } from '@/lib/api/types';
import type { Locale } from '@/i18n/locale';
import { formatCurrency, formatMonth, formatQuantity } from '@/lib/format';
import { useScopeParams, useSelection } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

const STATUS: Record<Bill['status'], Status> = {
  draft: 'neutral',
  issued: 'success',
  flagged: 'warning',
  superseded: 'neutral',
};

export type LatestBillCardViewProps = {
  bill: Bill | null;
  scopeLabel: string;
  loading?: boolean;
};

/** The most recent computed invoice for the chosen scope (01 §7.2). */
export function LatestBillCardView({ bill, scopeLabel, loading = false }: LatestBillCardViewProps) {
  const t = useTranslations('dashboard.bill');
  const status = useTranslations('domain.bill.status');
  const locale = useLocale() as Locale;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <p className="text-foreground-muted type-small">{scopeLabel}</p>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {loading ? (
          <>
            <Skeleton className="h-8 w-40" />
            <Skeleton className="h-4 w-28" />
          </>
        ) : !bill ? (
          <EmptyState icon={Receipt} title={t('empty')} description={t('emptyHint')} />
        ) : (
          <>
            <dl className="flex flex-col gap-2">
              <div className="flex items-baseline justify-between gap-2">
                <dt className="text-foreground-muted type-caption">{t('period')}</dt>
                <dd className="type-body">{formatMonth(bill.period_key, locale)}</dd>
              </div>
              <div className="flex items-baseline justify-between gap-2">
                <dt className="text-foreground-muted type-caption">{t('consumption')}</dt>
                <dd className="type-data">{formatQuantity(bill.active_import, 'kWh')}</dd>
              </div>
              <div className="flex items-baseline justify-between gap-2">
                <dt className="text-foreground-muted type-caption">{t('amount')}</dt>
                <dd className="type-metric">{formatCurrency(bill.total_cost)}</dd>
              </div>
            </dl>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <StatusBadge status={STATUS[bill.status]} label={status(bill.status)} />
              <Button asChild variant="secondary" size="sm">
                <Link href={`/ekorm/bills?bill=${bill.id}`}>{t('view')}</Link>
              </Button>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * Asks for the selected building's latest bill, or the company's when no
 * building is chosen and the role may see company-wide figures (R204).
 */
export function LatestBillCard({ buildingName }: { buildingName?: string }) {
  const t = useTranslations('dashboard.bill');
  const { me, can } = useSession();
  const scope = useScopeParams();
  const { buildingId } = useSelection();
  const companyWide = !buildingId && can('settings.company');
  const subject = buildingId ?? (companyWide ? (scope.company_id ?? me.company.id) : undefined);

  const query = $api.useQuery(
    'get',
    '/api/v1/bills/latest',
    { params: { query: { ...scope, scope: buildingId ? 'building' : 'company', subject_id: subject ?? '' } } },
    { enabled: Boolean(subject), retry: false, meta: { quietErrors: ['not_found'] } },
  );

  return (
    <LatestBillCardView
      bill={query.data ?? null}
      loading={query.isLoading}
      scopeLabel={buildingId && buildingName ? t('scopeBuilding', { name: buildingName }) : t('scopeCompany')}
    />
  );
}
