import type { LucideIcon } from 'lucide-react';

import type { Permission } from '@/lib/session/permissions';

import type { Messages } from '../../../messages';

type ShellNavKey = Exclude<keyof Messages['shell']['nav'], 'label'>;
type EkocmNavKey = keyof Messages['ekocm']['nav'];
/** A nav label is a full message path, so each module keeps its own label catalogue (R487). */
export type NavLabel = `shell.nav.${ShellNavKey}` | `ekocm.nav.${EkocmNavKey}`;

/** A leaf without `permission` needs `nav.core`; `navFor` filters by role. */
export type NavLeaf = { id: string; label: NavLabel; href: string; icon: LucideIcon; disabled?: boolean; permission?: Permission; exact?: boolean };
export type NavGroup = { id: string; label: NavLabel; icon: LucideIcon; children: NavLeaf[] };
export type NavEntry = NavLeaf | NavGroup;

export const isGroup = (entry: NavEntry): entry is NavGroup => 'children' in entry;

export const leavesOf = (entries: readonly NavEntry[]): NavLeaf[] => entries.flatMap((e) => (isGroup(e) ? e.children : [e]));

export function isActive(pathname: string, href: string, exact = false): boolean {
  return pathname === href || (!exact && pathname.startsWith(`${href}/`));
}

/** The group and leaf for a route (longest matching href, so /alarms/ai is not /alarms), for active state and breadcrumb. */
export function findTrail(entries: readonly NavEntry[], pathname: string): { group?: NavGroup; leaf: NavLeaf } | null {
  let best: { group?: NavGroup; leaf: NavLeaf } | null = null;
  for (const entry of entries) {
    for (const leaf of isGroup(entry) ? entry.children : [entry]) {
      if (isActive(pathname, leaf.href, leaf.exact) && leaf.href.length > (best?.leaf.href.length ?? -1)) {
        best = { group: isGroup(entry) ? entry : undefined, leaf };
      }
    }
  }
  return best;
}

/** The navigation a set of permissions may see (R159): leaves lacking their permission go, then empty groups. */
export function navFor(entries: readonly NavEntry[], permissions: readonly string[]): NavEntry[] {
  const allowed = (leaf: NavLeaf) => permissions.includes(leaf.permission ?? 'nav.core');
  return entries.flatMap((entry): NavEntry[] => {
    if (!isGroup(entry)) return allowed(entry) ? [entry] : [];
    const children = entry.children.filter(allowed);
    return children.length ? [{ ...entry, children }] : [];
  });
}
