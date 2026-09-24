'use client';

import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useState, type ReactNode } from 'react';

import { FilterBar } from '@/components/shell/filter-bar';
import { PageHeader } from '@/components/shell/page-header';
import { EmptyState } from '@/components/ui/empty-state';
import { Tabs } from '@/components/ui/tabs';
import { ScopePicker } from '@/features/scope/scope-picker';
import { useSelection } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

const TABS = ['summary', 'checklist'] as const;
export type IsoTab = (typeof TABS)[number];

const resolve = (v: string | null): IsoTab => ((TABS as readonly string[]).includes(v ?? '') ? (v as IsoTab) : 'summary');

/** What a view receives: the selected building and tab navigation. */
export type IsoContext = { buildingId: string; go: (tab: IsoTab) => void };

const VIEWS: Partial<Record<IsoTab, (ctx: IsoContext) => ReactNode>> = {};

/** 01 §7.17: the ISO 50001 workbench for the selected building (Q-G6). */
export function IsoPage() {
  const t = useTranslations('iso');
  const { can } = useSession();
  const { buildingId } = useSelection();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const active = resolve(params.get('tab'));
  const [activeOnly, setActiveOnly] = useState(false);
  const go = (tab: IsoTab) => {
    const next = new URLSearchParams(params.toString());
    next.set('tab', tab);
    router.replace(`${pathname}?${next.toString()}`);
  };

  if (!can('iso50001.read')) {
    return (
      <div className="flex flex-col gap-6">
        <PageHeader title={t('title')} description={t('subtitle')} />
        <EmptyState title={t('noAccess')} description={t('noAccessHint')} />
      </div>
    );
  }
  const body = buildingId ? (VIEWS[active]?.({ buildingId, go }) ?? null) : (
    <EmptyState title={t('chooseBuilding')} description={t('chooseBuildingHint')} />
  );
  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />
      <FilterBar>
        <ScopePicker activeOnly={activeOnly} onActiveOnlyChange={setActiveOnly} />
      </FilterBar>
      <Tabs
        value={active}
        onValueChange={(v) => go(resolve(v))}
        items={TABS.map((tab) => ({ value: tab, label: t(`tabs.${tab}`), content: active === tab ? body : null }))}
      />
      <footer className="flex flex-col gap-1 border-t border-border pt-4 text-foreground-muted type-small">
        <p>{t('footer.standard')}</p>
        <p>{t('footer.guidance')}</p>
      </footer>
    </div>
  );
}
