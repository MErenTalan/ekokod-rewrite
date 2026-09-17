'use client';

import { SlidersHorizontal } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useState, type ReactNode } from 'react';

import { Button } from '../ui/button';
import { Drawer } from '../ui/drawer';

export type FilterBarProps = { children: ReactNode; onApply?: () => void; activeCount?: number };

/** Sticky under the top bar; below sm the filters move into a bottom sheet (07 §7). */
export function FilterBar({ children, onApply, activeCount }: FilterBarProps) {
  const t = useTranslations('shell');
  const [open, setOpen] = useState(false);
  return (
    <div className="sticky top-14 z-10 -mx-4 border-b border-border bg-background px-4 py-3 lg:-mx-6 lg:px-6">
      <div className="hidden flex-wrap items-end gap-3 sm:flex">
        {children}
        {onApply ? <Button onClick={onApply}>{t('applyFilters')}</Button> : null}
      </div>
      <div className="sm:hidden">
        <Button variant="secondary" iconStart={SlidersHorizontal} onClick={() => setOpen(true)}>
          {activeCount ? t('filtersCount', { count: activeCount }) : t('filters')}
        </Button>
      </div>
      {open ? (
        <Drawer
          open
          side="bottom"
          title={t('filters')}
          onOpenChange={setOpen}
          footer={
            onApply ? (
              <Button
                data-apply
                onClick={() => {
                  onApply();
                  setOpen(false);
                }}
              >
                {t('applyFilters')}
              </Button>
            ) : undefined
          }
        >
          <div className="flex flex-col gap-3">{children}</div>
        </Drawer>
      ) : null}
    </div>
  );
}
