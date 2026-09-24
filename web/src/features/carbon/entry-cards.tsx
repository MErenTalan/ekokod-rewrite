'use client';

import { useTranslations } from 'next-intl';

import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import type { CarbonCatalogueMain } from '@/lib/api/types';

import { isoKey, mainKey, scopeKey, subKey } from './labels';

export type EntrySub = CarbonCatalogueMain['subs'][number] & { main: string };

export type EntryCardsProps = {
  subs: EntrySub[];
  counts: Record<string, number>;
  /** The count query hit its limit, so every count is a lower bound. */
  full: boolean;
  editable: boolean;
  onAdd: (sub: EntrySub) => void;
  onGoSelection: () => void;
};

/** R324: one card per declared sub-category. */
export function EntryCards({ subs, counts, full, editable, onAdd, onGoSelection }: EntryCardsProps) {
  const t = useTranslations('carbon');
  if (subs.length === 0) {
    return (
      <EmptyState
        title={t('entry.nothingSelected')}
        description={t('entry.nothingSelectedHint')}
        action={<Button onClick={onGoSelection}>{t('entry.goSelection')}</Button>}
      />
    );
  }
  return (
    <div className="flex flex-col gap-4">
      <p className="text-foreground-muted type-body">{editable ? t('entry.intro') : t('entry.readOnly')}</p>
      <ul className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {subs.map((sub) => (
          <li key={sub.key} className="flex flex-col gap-2 rounded-lg border border-border bg-surface-raised p-4">
            <p className="text-foreground-muted type-caption">{t(mainKey(sub.main))}</p>
            <h3 className="type-h3">{t(subKey(sub.key))}</h3>
            <p className="text-foreground-muted type-small">
              {t('selection.mapping', { scope: t(scopeKey(sub.scope)), iso: t(isoKey(sub.iso_category)) })}
            </p>
            <p className="text-foreground type-small">{full ? t('entry.recordsMany') : t('entry.records', { count: counts[sub.key] ?? 0 })}</p>
            {editable ? (
              <Button variant="secondary" size="sm" className="self-start" onClick={() => onAdd(sub)}>
                {t('entry.add')}
              </Button>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}
