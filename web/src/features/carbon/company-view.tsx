'use client';

import Link from 'next/link';
import { useTranslations } from 'next-intl';
import type { ReactNode } from 'react';

import { Button } from '@/components/ui/button';
import type { Company } from '@/lib/api/types';
import { formatNumber } from '@/lib/format';

export type CompanyViewProps = {
  /** Absent for a role that cannot read the company record (Q-F10). */
  company?: Company;
  companyName: string;
  canEdit: boolean;
};

/** §7.16 Company Details: what the carbon report header prints. */
export function CompanyView({ company, companyName, canEdit }: CompanyViewProps) {
  const t = useTranslations('carbon');
  if (!company) {
    return (
      <div className="flex flex-col gap-2">
        <p className="text-foreground-muted type-body">{t('company.restricted')}</p>
        <p className="type-h3">{companyName}</p>
      </div>
    );
  }
  const missing = <span className="text-foreground-muted">{t('company.missing')}</span>;
  const rows: [string, ReactNode][] = [
    [t('company.name'), company.name],
    [t('company.address'), company.address || missing],
    [t('company.sector'), company.sector || missing],
    [t('company.personnel'), company.personnel_count != null ? formatNumber(company.personnel_count) : missing],
    [t('company.area'), company.total_area_m2 ? formatNumber(company.total_area_m2) : missing],
    [t('company.contact'), [company.contact_name, company.contact_phone].filter(Boolean).join(' · ') || missing],
  ];
  return (
    <div className="flex flex-col gap-4">
      <p className="text-foreground-muted type-body">{t('company.intro')}</p>
      <dl className="grid gap-x-6 gap-y-3 rounded-lg border border-border bg-surface-raised p-4 sm:grid-cols-[max-content_1fr]">
        {rows.map(([label, value]) => (
          <div key={label} className="contents">
            <dt className="text-foreground-muted type-small">{label}</dt>
            <dd className="text-foreground type-body">{value}</dd>
          </div>
        ))}
      </dl>
      {canEdit ? (
        <Button asChild variant="secondary" size="sm" className="self-start">
          <Link href="/ekorm/settings?tab=company">{t('company.edit')}</Link>
        </Button>
      ) : null}
    </div>
  );
}
