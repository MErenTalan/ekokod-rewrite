import {
  Activity,
  BadgeCheck,
  Bell,
  BellRing,
  CalendarDays,
  Droplets,
  Factory,
  FileBarChart,
  FileText,
  Flame,
  LayoutDashboard,
  Leaf,
  LineChart,
  Mail,
  MessageSquare,
  PiggyBank,
  PlugZap,
  Receipt,
  Settings,
  Sparkles,
  Sun,
  Tags,
  TrendingUp,
  Wallet,
  Zap,
  type LucideIcon,
} from 'lucide-react';

import type { Permission } from '@/lib/session/permissions';

import type { Messages } from '../../../messages';

/** Sidebar information architecture (07 §7, plan D18, R167). A leaf without `permission` needs `nav.core`; `navFor` filters by role. */
type NavKey = Exclude<keyof Messages['shell']['nav'], 'label'>;
export type NavLeaf = { id: string; labelKey: NavKey; href: string; icon: LucideIcon; disabled?: boolean; permission?: Permission; exact?: boolean };
export type NavGroup = { id: string; labelKey: NavKey; icon: LucideIcon; children: NavLeaf[] };
export type NavEntry = NavLeaf | NavGroup;
export const navigation: NavEntry[] = [
  { id: 'dashboard', labelKey: 'dashboard', href: '/ekorm', icon: LayoutDashboard, exact: true },
  { id: 'dataAnalysis', labelKey: 'dataAnalysis', icon: LineChart, children: [
    { id: 'consumption', labelKey: 'consumption', href: '/ekorm/consumption', icon: Zap },
    { id: 'loadProfile', labelKey: 'loadProfile', href: '/ekorm/load-profile', icon: Activity },
    { id: 'forecast', labelKey: 'forecast', href: '/ekorm/forecast', icon: TrendingUp },
    { id: 'solarPlants', labelKey: 'solarPlants', href: '/ekorm/solar-plants', icon: Sun, permission: 'nav.solar_plants' },
    { id: 'financial', labelKey: 'financialAnalysis', href: '/ekorm/financial', icon: Wallet, permission: 'nav.financial' },
    { id: 'renewable', labelKey: 'renewableEnergy', href: '/ekorm/renewable', icon: Leaf },
    { id: 'water', labelKey: 'water', href: '/ekorm/water', icon: Droplets, disabled: true },
    { id: 'gas', labelKey: 'gas', href: '/ekorm/gas', icon: Flame, disabled: true },
    { id: 'evDrivers', labelKey: 'evDrivers', href: '/ekorm/ev-drivers', icon: PlugZap, disabled: true } ] },
  { id: 'billsTariffs', labelKey: 'billsAndTariffs', icon: Receipt, children: [
    { id: 'bills', labelKey: 'bills', href: '/ekorm/bills', icon: FileText, permission: 'bills.read' },
    { id: 'tariffs', labelKey: 'tariffs', href: '/ekorm/tariffs', icon: Tags, permission: 'tariffs.read' } ] },
  { id: 'alarms', labelKey: 'alarms', icon: Bell, children: [
    { id: 'alarmsManual', labelKey: 'manual', href: '/ekorm/alarms', icon: BellRing, permission: 'alarms.read' },
    { id: 'messages', labelKey: 'messages', href: '/ekorm/messages', icon: MessageSquare, permission: 'messages.read' },
    { id: 'alarmsAi', labelKey: 'ai', href: '/ekorm/alarms/ai', icon: Sparkles, disabled: true } ] },
  { id: 'reports', labelKey: 'reports', href: '/ekorm/reports', icon: FileBarChart },
  { id: 'calendar', labelKey: 'calendar', href: '/ekorm/calendar', icon: CalendarDays },
  { id: 'settings', labelKey: 'settings', href: '/ekorm/settings', icon: Settings },
  { id: 'carbon', labelKey: 'carbonFootprint', href: '/ekorm/carbon', icon: Factory },
  { id: 'iso50001', labelKey: 'iso50001', href: '/ekorm/iso-50001', icon: BadgeCheck },
  { id: 'savingActions', labelKey: 'savingActions', href: '/ekorm/saving-actions', icon: PiggyBank, disabled: true },
  { id: 'contact', labelKey: 'contact', href: '/ekorm/contact', icon: Mail },
];

export const isGroup = (entry: NavEntry): entry is NavGroup => 'children' in entry;

export function isActive(pathname: string, href: string, exact = false): boolean {
  return pathname === href || (!exact && pathname.startsWith(`${href}/`));
}

/** The group and leaf for a route (longest matching href, so /alarms/ai is not /alarms), for active state and breadcrumb. */
export function findTrail(pathname: string): { group?: NavGroup; leaf: NavLeaf } | null {
  let best: { group?: NavGroup; leaf: NavLeaf } | null = null;
  for (const entry of navigation) {
    for (const leaf of isGroup(entry) ? entry.children : [entry]) {
      if (isActive(pathname, leaf.href, leaf.exact) && leaf.href.length > (best?.leaf.href.length ?? -1)) {
        best = { group: isGroup(entry) ? entry : undefined, leaf };
      }
    }
  }
  return best;
}

/** The navigation a set of permissions may see (R159): leaves lacking their permission go, then empty groups. */
export function navFor(permissions: readonly string[]): NavEntry[] {
  const allowed = (leaf: NavLeaf) => permissions.includes(leaf.permission ?? 'nav.core');
  return navigation.flatMap((entry): NavEntry[] => {
    if (!isGroup(entry)) return allowed(entry) ? [entry] : [];
    const children = entry.children.filter(allowed);
    return children.length ? [{ ...entry, children }] : [];
  });
}
