'use client';

import { useTranslations } from 'next-intl';

import { $api } from '@/lib/api/query';
import { useSelection } from '@/lib/selection/selection-store';
import { useOptionalSession } from '@/lib/session/session-provider';

import { Combobox } from '../ui/combobox';

export type CompanyOption = { id: string; name: string };

/** The admin's company scope (R139): choosing another company scopes every page to it; the own company clears it. */
export function CompanySwitcherView({
  own,
  companies,
  value,
  onValueChange,
}: {
  own: CompanyOption;
  companies: CompanyOption[];
  value: string | undefined;
  onValueChange: (companyId: string | undefined) => void;
}) {
  const t = useTranslations('shell.companySwitcher');
  const options = [
    { value: own.id, label: t('own', { name: own.name }) },
    ...companies.filter((c) => c.id !== own.id).map((c) => ({ value: c.id, label: c.name })),
  ];
  return (
    <div className="w-56">
      <Combobox
        label={t('label')}
        labelVisibility="hidden"
        options={options}
        value={value ?? own.id}
        onValueChange={(v) => onValueChange(!v || v === own.id ? undefined : v)}
        searchPlaceholder={t('search')}
        emptyText={t('empty')}
      />
    </div>
  );
}

export function CompanySwitcher() {
  const session = useOptionalSession();
  const { companyId, set } = useSelection();
  const enabled = Boolean(session?.can('admin.companies'));
  const { data } = $api.useQuery('get', '/api/v1/companies', { params: { query: { limit: 500 } } }, { enabled });
  if (!session || !enabled) return null;
  return (
    <CompanySwitcherView
      own={session.me.company}
      companies={data?.items ?? []}
      value={companyId}
      onValueChange={(id) => set({ companyId: id })}
    />
  );
}
