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

import { OverviewPanel } from './overview-panel';
import { SelectionPanel } from './selection-panel';
import { NEEDS_BUILDING, resolveTab, TABS, type CarbonTab } from './labels';

/** What a section receives: the selected building (R321) and tab navigation. */
export type SectionContext = { buildingId: string; go: (tab: CarbonTab) => void };

/** Each section's content, keyed by tab. */
const SECTIONS: Partial<Record<CarbonTab, (ctx: SectionContext) => ReactNode>> = {
  overview: (ctx) => <OverviewPanel {...ctx} />,
  selection: ({ buildingId }) => <SelectionPanel buildingId={buildingId} />,
};

/** Eko-CM (01 §7.16): one module, its sections as deep-linkable tabs (R320, Q-F8). */
export function CarbonPage() {
  const t = useTranslations('carbon');
  const { can } = useSession();
  const { buildingId } = useSelection();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const active = resolveTab(params.get('tab'));
  const [activeOnly, setActiveOnly] = useState(false);

  const go = (tab: CarbonTab) => {
    const next = new URLSearchParams(params.toString());
    next.set('tab', tab);
    router.replace(`${pathname}?${next.toString()}`);
  };

  if (!can('carbon.read')) {
    return (
      <div className="flex flex-col gap-6">
        <PageHeader title={t('title')} description={t('subtitle')} />
        <EmptyState title={t('noAccess')} description={t('noAccessHint')} />
      </div>
    );
  }

  const needsBuilding = NEEDS_BUILDING.has(active);
  const body =
    needsBuilding && !buildingId ? (
      <EmptyState title={t('chooseBuilding')} description={t('chooseBuildingHint')} />
    ) : (
      (SECTIONS[active]?.({ buildingId: buildingId ?? '', go }) ?? null)
    );

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />
      {needsBuilding ? (
        <FilterBar>
          <ScopePicker activeOnly={activeOnly} onActiveOnlyChange={setActiveOnly} />
        </FilterBar>
      ) : null}
      <Tabs
        value={active}
        onValueChange={(v) => go(resolveTab(v))}
        items={TABS.map((tab) => ({ value: tab, label: t(`tabs.${tab}`), content: active === tab ? body : null }))}
      />
    </div>
  );
}
