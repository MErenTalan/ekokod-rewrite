// Dependency-free: the middleware maps legacy `?tab=` links with it (R493).
import type { CarbonTab } from '@/features/carbon/labels';

/** R488: each former `?tab=` section is a page of its own. */
export const SECTION_HREF: Record<CarbonTab, string> = {
  overview: '/ekocm',
  selection: '/ekocm/activity-selection',
  entry: '/ekocm/data-entry',
  status: '/ekocm/activity-status',
  reporting: '/ekocm/reports',
  database: '/ekocm/emission-factors',
  company: '/ekocm/company',
  ghg: '/ekocm/ghg-protocol',
  iso: '/ekocm/iso-14064',
  standards: '/ekocm/standards',
  documents: '/ekocm/documents',
};

/** The page for a legacy `?tab=` value; anything unknown is the overview (R320, R493). */
export function sectionHref(tab: string | null): string {
  return tab && Object.hasOwn(SECTION_HREF, tab) ? SECTION_HREF[tab as CarbonTab] : SECTION_HREF.overview;
}
