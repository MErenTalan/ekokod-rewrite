import { Factory, Leaf, type LucideIcon } from 'lucide-react';

import { findTrail, type NavEntry } from '@/components/shell/nav';

import { ekocmNav } from './ekocm/nav';
import { ekormNav } from './ekorm/nav';
import { MODULE_IDS, type ModuleId } from './ids';

export { MODULE_IDS, type ModuleId };

/** `iconClass` colours the module's mark outside its own shell (the chooser), where `stroke-brand` is not rebound. */
export type ModuleDef = { id: ModuleId; prefix: `/${ModuleId}`; home: string; nav: NavEntry[]; icon: LucideIcon; iconClass: string };

/** R485: the frontend modules; backend, auth and session are shared by all of them. */
export const MODULES: Record<ModuleId, ModuleDef> = {
  ekorm: { id: 'ekorm', prefix: '/ekorm', home: '/ekorm', nav: ekormNav, icon: Leaf, iconClass: 'stroke-brand' },
  ekocm: { id: 'ekocm', prefix: '/ekocm', home: '/ekocm', nav: ekocmNav, icon: Factory, iconClass: 'stroke-module-ekocm' },
};

/** The module a path belongs to, or null outside every module. */
export function moduleFor(pathname: string): ModuleDef | null {
  for (const id of MODULE_IDS) {
    const { prefix } = MODULES[id];
    if (pathname === prefix || pathname.startsWith(`${prefix}/`)) return MODULES[id];
  }
  return null;
}

/** The module a shell piece renders for: the path's, else EKORM (isolated stories and tests). */
export const shellModule = (pathname: string): ModuleDef => moduleFor(pathname) ?? MODULES.ekorm;

/** The nav trail of a path inside its own module (active state, breadcrumb). */
export function trailFor(pathname: string) {
  return findTrail(shellModule(pathname).nav, pathname);
}
