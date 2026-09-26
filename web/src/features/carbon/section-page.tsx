'use client';

import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useState, type ReactNode } from 'react';

import { FilterBar } from '@/components/shell/filter-bar';
import { PageHeader } from '@/components/shell/page-header';
import { EmptyState } from '@/components/ui/empty-state';
import { ScopePicker } from '@/features/scope/scope-picker';
import { useSelection } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';
import { SECTION_HREF } from '@/modules/ekocm/sections';

import { CompanyPanel } from './company-panel';
import { DatabasePanel } from './database-panel';
import { DocumentsView } from './documents-view';
import { EntryPanel } from './entry-panel';
import { OverviewPanel } from './overview-panel';
import { ReferencePanel } from './reference-panel';
import { ReportingPanel } from './reporting-panel';
import { SelectionPanel } from './selection-panel';
import { StatusPanel } from './status-panel';
import { NEEDS_BUILDING, type CarbonTab } from './labels';

/** What a section receives: the selected building (R321) and navigation to a sibling section. */
export type SectionContext = { buildingId: string; go: (tab: CarbonTab) => void };

const SECTIONS: Record<CarbonTab, (ctx: SectionContext) => ReactNode> = {
  overview: (ctx) => <OverviewPanel {...ctx} />,
  selection: ({ buildingId }) => <SelectionPanel buildingId={buildingId} />,
  entry: (ctx) => <EntryPanel {...ctx} />,
  status: ({ buildingId }) => <StatusPanel buildingId={buildingId} />,
  reporting: ({ buildingId }) => <ReportingPanel buildingId={buildingId} />,
  database: () => <DatabasePanel />,
  company: () => <CompanyPanel />,
  ghg: () => <ReferencePanel kind="ghg" />,
  iso: () => <ReferencePanel kind="iso" />,
  standards: () => <ReferencePanel kind="standards" />,
  documents: () => <DocumentsView />,
};

/** One EKOCM page (R488): the carbon.read gate, the building requirement (R321) and the section itself. */
export function CarbonSection({ section }: { section: CarbonTab }) {
  const t = useTranslations('carbon');
  const { can } = useSession();
  const { buildingId } = useSelection();
  const router = useRouter();
  const [activeOnly, setActiveOnly] = useState(false);

  if (!can('carbon.read')) {
    return (
      <div className="flex flex-col gap-6">
        <PageHeader title={t('title')} description={t('subtitle')} />
        <EmptyState title={t('noAccess')} description={t('noAccessHint')} />
      </div>
    );
  }

  const needsBuilding = NEEDS_BUILDING.has(section);
  const go = (tab: CarbonTab) => router.push(SECTION_HREF[tab]);
  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t(`tabs.${section}`)} description={section === 'overview' ? t('subtitle') : undefined} />
      {needsBuilding ? (
        <FilterBar>
          <ScopePicker activeOnly={activeOnly} onActiveOnlyChange={setActiveOnly} />
        </FilterBar>
      ) : null}
      {needsBuilding && !buildingId ? (
        <EmptyState title={t('chooseBuilding')} description={t('chooseBuildingHint')} />
      ) : (
        SECTIONS[section]({ buildingId: buildingId ?? '', go })
      )}
    </div>
  );
}
