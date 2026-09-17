import {
  Activity,
  BadgeCheck,
  Bell,
  BellRing,
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

import type { Messages } from '../../../messages';

/** Sidebar information architecture (07 §7, plan D18). Data, so F6 edits one array (and filters it by role). */
type NavKey = Exclude<keyof Messages['shell']['nav'], 'label'>;
export type NavLeaf = { id: string; labelKey: NavKey; href: string; icon: LucideIcon; disabled?: boolean };
export type NavGroup = { id: string; labelKey: NavKey; icon: LucideIcon; children: NavLeaf[] };
export type NavEntry = NavLeaf | NavGroup;
export const navigation: NavEntry[] = [
  { id: 'dashboard', labelKey: 'dashboard', href: '/dashboard', icon: LayoutDashboard },
  { id: 'dataAnalysis', labelKey: 'dataAnalysis', icon: LineChart, children: [
    { id: 'consumption', labelKey: 'consumption', href: '/consumption', icon: Zap },
    { id: 'loadProfile', labelKey: 'loadProfile', href: '/load-profile', icon: Activity },
    { id: 'forecast', labelKey: 'forecast', href: '/forecast', icon: TrendingUp },
    { id: 'solarPlants', labelKey: 'solarPlants', href: '/solar-plants', icon: Sun },
    { id: 'financial', labelKey: 'financialAnalysis', href: '/financial', icon: Wallet },
    { id: 'renewable', labelKey: 'renewableEnergy', href: '/renewable', icon: Leaf },
    { id: 'water', labelKey: 'water', href: '/water', icon: Droplets, disabled: true },
    { id: 'gas', labelKey: 'gas', href: '/gas', icon: Flame, disabled: true },
    { id: 'evDrivers', labelKey: 'evDrivers', href: '/ev-drivers', icon: PlugZap, disabled: true } ] },
  { id: 'billsTariffs', labelKey: 'billsAndTariffs', icon: Receipt, children: [
    { id: 'bills', labelKey: 'bills', href: '/bills', icon: FileText }, { id: 'tariffs', labelKey: 'tariffs', href: '/tariffs', icon: Tags } ] },
  { id: 'alarms', labelKey: 'alarms', icon: Bell, children: [
    { id: 'alarmsManual', labelKey: 'manual', href: '/alarms', icon: BellRing }, { id: 'messages', labelKey: 'messages', href: '/messages', icon: MessageSquare },
    { id: 'alarmsAi', labelKey: 'ai', href: '/alarms/ai', icon: Sparkles, disabled: true } ] },
  { id: 'reports', labelKey: 'reports', href: '/reports', icon: FileBarChart },
  { id: 'settings', labelKey: 'settings', href: '/settings', icon: Settings },
  { id: 'carbon', labelKey: 'carbonFootprint', href: '/carbon', icon: Factory },
  { id: 'iso50001', labelKey: 'iso50001', href: '/iso-50001', icon: BadgeCheck },
  { id: 'savingActions', labelKey: 'savingActions', href: '/saving-actions', icon: PiggyBank, disabled: true },
  { id: 'contact', labelKey: 'contact', href: '/contact', icon: Mail },
];

export const isGroup = (entry: NavEntry): entry is NavGroup => 'children' in entry;

export function isActive(pathname: string, href: string): boolean {
  return pathname === href || pathname.startsWith(`${href}/`);
}

/** The group and leaf for a route (longest matching href, so /alarms/ai is not /alarms), for active state and breadcrumb. */
export function findTrail(pathname: string): { group?: NavGroup; leaf: NavLeaf } | null {
  let best: { group?: NavGroup; leaf: NavLeaf } | null = null;
  for (const entry of navigation) {
    for (const leaf of isGroup(entry) ? entry.children : [entry]) {
      if (isActive(pathname, leaf.href) && leaf.href.length > (best?.leaf.href.length ?? -1)) {
        best = { group: isGroup(entry) ? entry : undefined, leaf };
      }
    }
  }
  return best;
}
