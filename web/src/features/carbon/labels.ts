// API keys (R300/R301, snake_case) → `carbon` message keys (camelCase).

const camel = (s: string) => s.replace(/_([a-z0-9])/g, (_, c: string) => c.toUpperCase());

/** `cat_stationary` → `mains.stationary`. */
export const mainKey = (key: string) => `mains.${camel(key.replace(/^cat_/, ''))}` as const;
/** `sub_space_heating` → `subs.spaceHeating`. */
export const subKey = (key: string) => `subs.${camel(key.replace(/^sub_/, ''))}` as const;
/** `scope_1` → `scopes.scope1`. */
export const scopeKey = (key: string) => `scopes.${camel(key)}` as const;
/** `category_6` → `iso.category6`. */
export const isoKey = (key: string) => `iso.${camel(key)}` as const;

export const TABS = ['overview', 'selection', 'entry', 'status', 'reporting', 'database', 'company', 'ghg', 'iso', 'standards', 'documents'] as const;
export type CarbonTab = (typeof TABS)[number];

/** R321: these sections act on one building. */
export const NEEDS_BUILDING: ReadonlySet<CarbonTab> = new Set(['overview', 'selection', 'entry', 'status', 'reporting']);

export function resolveTab(value: string | null): CarbonTab {
  return (TABS as readonly string[]).includes(value ?? '') ? (value as CarbonTab) : 'overview';
}

/** Activity status as colour + icon + text (07 §2.4). */
export const STATUS_TONE = { pending: 'neutral', approved: 'success', rejected: 'danger' } as const;

/** kg → t with the API's decimal string kept exact up to the division. */
export const toTonnes = (kg: string) => Number(kg) / 1000;
