'use client';

import { useTranslations } from 'next-intl';

import { MultiSelect } from '@/components/ui/multi-select';
import type { LoadProfiles, Translator } from '@/lib/api/types';

import { ProfileChart } from './profile-chart';
import { PROFILE_KEYS, profileLabel, type ProfileKey } from './profile-label';

/** More than six series stop being readable (plan D21), so the picker stops there. */
export const MAX_COMPARED = 6;

export type CompareTabViewProps = {
  data: LoadProfiles | null;
  selected: ProfileKey[];
  onSelectedChange: (keys: ProfileKey[]) => void;
  loading?: boolean;
};

/** The Compare tab: any subset of the ten profiles on one 24-hour axis. */
export function CompareTabView({ data, selected, onSelectedChange, loading = false }: CompareTabViewProps) {
  const t = useTranslations('loadProfile') as unknown as Translator;
  const compare = useTranslations('loadProfile.compare');
  const full = selected.length >= MAX_COMPARED;
  return (
    <div className="flex flex-col gap-4">
      <div className="max-w-xl">
        <MultiSelect
          label={compare('select')}
          description={full ? compare('max') : undefined}
          options={PROFILE_KEYS.map((key) => ({
            value: key,
            label: profileLabel(t, key),
            disabled: full && !selected.includes(key),
          }))}
          value={selected}
          onValueChange={(keys) => onSelectedChange(keys.slice(0, MAX_COMPARED) as ProfileKey[])}
          searchPlaceholder={compare('search')}
          emptyText={compare('empty')}
        />
      </div>
      <ProfileChart data={data} keys={selected} title={compare('title')} loading={loading} />
    </div>
  );
}
