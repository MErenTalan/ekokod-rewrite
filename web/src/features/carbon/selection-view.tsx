'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import type { CarbonCatalogue } from '@/lib/api/types';

import { isoKey, mainKey, scopeKey, subKey } from './labels';

export type SelectionViewProps = {
  catalogue: CarbonCatalogue;
  selected: string[];
  editable: boolean;
  saving?: boolean;
  onSave: (keys: string[]) => void;
};

/** R323: the building declares which sub-categories apply to it. */
export function SelectionView({ catalogue, selected, editable, saving = false, onSave }: SelectionViewProps) {
  const t = useTranslations('carbon');
  const [checked, setChecked] = useState(() => new Set(selected));
  const toggle = (key: string, on: boolean) =>
    setChecked((prev) => {
      const next = new Set(prev);
      if (on) next.add(key);
      else next.delete(key);
      return next;
    });

  return (
    <div className="flex flex-col gap-6">
      <p className="text-foreground-muted type-body">{editable ? t('selection.intro') : t('selection.readOnly')}</p>
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {catalogue.items.map((main) => (
          <fieldset
            key={main.key}
            role="group"
            aria-label={t(mainKey(main.key))}
            className="flex flex-col gap-3 rounded-lg border border-border bg-surface-raised p-4"
          >
            <legend className="px-1 type-h3">{t(mainKey(main.key))}</legend>
            {main.subs.map((sub) => (
              <Checkbox
                key={sub.key}
                label={t(subKey(sub.key))}
                description={t('selection.mapping', { scope: t(scopeKey(sub.scope)), iso: t(isoKey(sub.iso_category)) })}
                checked={checked.has(sub.key)}
                disabled={!editable}
                onCheckedChange={(on) => toggle(sub.key, on)}
              />
            ))}
          </fieldset>
        ))}
      </div>
      {editable ? (
        <div className="flex flex-wrap items-center gap-3">
          <Button loading={saving} onClick={() => onSave([...checked].sort())}>
            {t('selection.save')}
          </Button>
          <span className="text-foreground-muted type-small">{t('selection.selectedCount', { count: checked.size })}</span>
        </div>
      ) : null}
    </div>
  );
}
