import {
  Award,
  BookOpen,
  Building2,
  ClipboardCheck,
  ClipboardList,
  Database,
  FileBarChart,
  FileText,
  Globe,
  LayoutDashboard,
  Library,
  ListChecks,
  PencilLine,
  Scale,
} from 'lucide-react';

import type { NavEntry } from '@/components/shell/nav';
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

const p = 'carbon.read' as const;

/** EKOCM's sidebar (R488); every leaf needs carbon.read. */
export const ekocmNav: NavEntry[] = [
  { id: 'overview', label: 'ekocm.nav.overview', href: SECTION_HREF.overview, icon: LayoutDashboard, permission: p, exact: true },
  { id: 'data', label: 'ekocm.nav.data', icon: ClipboardList, children: [
    { id: 'selection', label: 'ekocm.nav.selection', href: SECTION_HREF.selection, icon: ListChecks, permission: p },
    { id: 'entry', label: 'ekocm.nav.entry', href: SECTION_HREF.entry, icon: PencilLine, permission: p },
    { id: 'status', label: 'ekocm.nav.status', href: SECTION_HREF.status, icon: ClipboardCheck, permission: p } ] },
  { id: 'reporting', label: 'ekocm.nav.reporting', href: SECTION_HREF.reporting, icon: FileBarChart, permission: p },
  { id: 'library', label: 'ekocm.nav.library', icon: Library, children: [
    { id: 'database', label: 'ekocm.nav.emissionFactors', href: SECTION_HREF.database, icon: Database, permission: p },
    { id: 'company', label: 'ekocm.nav.company', href: SECTION_HREF.company, icon: Building2, permission: p } ] },
  { id: 'reference', label: 'ekocm.nav.reference', icon: BookOpen, children: [
    { id: 'ghg', label: 'ekocm.nav.ghg', href: SECTION_HREF.ghg, icon: Globe, permission: p },
    { id: 'iso', label: 'ekocm.nav.iso', href: SECTION_HREF.iso, icon: Award, permission: p },
    { id: 'standards', label: 'ekocm.nav.standards', href: SECTION_HREF.standards, icon: Scale, permission: p },
    { id: 'documents', label: 'ekocm.nav.documents', href: SECTION_HREF.documents, icon: FileText, permission: p } ] },
];
