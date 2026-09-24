'use client';

import Link from 'next/link';
import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { RadioGroup } from '@/components/ui/radio-group';
import { Select } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import type { ISolarAccountPlant } from '@/lib/api/types';
import { formatNumber } from '@/lib/format';

export type IsolarLinkViewProps = {
  plantId: string;
  credentials: { value: string; label: string }[];
  credentialId: string | null;
  onCredentialChange: (id: string) => void;
  plants: ISolarAccountPlant[];
  loading?: boolean;
  linking?: boolean;
  onLink: (psId: string) => void;
};

/**
 * 01 §7.15's iSolar plant link modal (R281): the account's plants, with the
 * ones another plant already holds disabled, since a ps id links once.
 */
export function IsolarLinkView({ plantId, credentials, credentialId, onCredentialChange, plants, loading = false, linking = false, onLink }: IsolarLinkViewProps) {
  const t = useTranslations('settings.plants');
  const [chosen, setChosen] = useState<string | null>(null);

  if (credentials.length === 0) {
    return (
      <EmptyState title={t('link.noCredential')} description={t('link.noCredentialDescription')}
        action={
          <Button asChild size="sm" variant="secondary">
            <Link href="/ekorm/settings?tab=company">{t('link.goToCompany')}</Link>
          </Button>
        } />
    );
  }

  const options = plants.map((p) => {
    const elsewhere = Boolean(p.linked_plant_id) && p.linked_plant_id !== plantId;
    const here = p.linked_plant_id === plantId;
    const kw = p.installed_kw ? ` · ${t('link.installed', { kw: formatNumber(p.installed_kw) })}` : '';
    const note = elsewhere ? ` — ${t('link.linkedElsewhere')}` : here ? ` — ${t('link.linkedHere')}` : '';
    return { value: p.ps_id, label: `${p.name}${kw}${note}`, disabled: elsewhere };
  });

  return (
    <div className="flex flex-col gap-4">
      <Select label={t('link.credential')} options={credentials} value={credentialId} onValueChange={onCredentialChange} />
      {loading ? (
        <Skeleton className="h-32 w-full" />
      ) : plants.length === 0 ? (
        <EmptyState title={t('link.empty')} description={t('link.emptyDescription')} />
      ) : (
        <RadioGroup label={t('link.plants')} options={options} value={chosen ?? ''} onValueChange={setChosen} />
      )}
      <div className="flex justify-end">
        <Button disabled={!chosen} loading={linking} onClick={() => chosen && onLink(chosen)}>{t('link.confirm')}</Button>
      </div>
    </div>
  );
}
