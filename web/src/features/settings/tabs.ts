import type { Permission } from '@/lib/session/permissions';

/** The eight tabs of 01 §7.15, in the order that section lists them. */
export const SETTINGS_TABS = [
  { id: 'account', permission: null },
  { id: 'integrations', permission: 'settings.integrations' },
  { id: 'company', permission: 'settings.company' },
  { id: 'buildings', permission: 'settings.buildings' },
  { id: 'plants', permission: 'settings.plants' },
  { id: 'analyzers', permission: 'settings.analyzers' },
  { id: 'users', permission: 'settings.users' },
  { id: 'smtp', permission: 'settings.smtp' },
] as const satisfies readonly { id: string; permission: Permission | null }[];

export type SettingsTabId = (typeof SETTINGS_TABS)[number]['id'];

/** The tabs a permission set may see (01 §2's visibility matrix, R200). */
export function visibleTabs(can: (permission: Permission) => boolean): SettingsTabId[] {
  return SETTINGS_TABS.filter((tab) => tab.permission === null || can(tab.permission)).map((tab) => tab.id);
}

/** An unknown or forbidden `?tab=` falls back to Account, which everyone has. */
export function resolveTab(requested: string | null, visible: SettingsTabId[]): SettingsTabId {
  return visible.includes(requested as SettingsTabId) ? (requested as SettingsTabId) : 'account';
}
