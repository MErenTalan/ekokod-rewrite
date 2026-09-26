import { sectionHref } from './ekocm/sections';

/** Legacy root paths (legacy-inventory §1) → where they live now; `keepRest` carries sub-paths over. */
export const LEGACY_ROOTS: Record<string, { to: string; keepRest: boolean }> = {
  consumption: { to: '/ekorm/consumption', keepRest: true },
  'load-profile': { to: '/ekorm/load-profile', keepRest: true },
  predict: { to: '/ekorm/predict', keepRest: true },
  ai: { to: '/ekorm/ai', keepRest: true },
  'solar-plants': { to: '/ekorm/solar-plants', keepRest: true },
  'renewable-energy': { to: '/ekorm/renewable-energy', keepRest: true },
  'financial-analysis': { to: '/ekorm/financial-analysis', keepRest: true },
  bills: { to: '/ekorm/bills', keepRest: true },
  'bills-and-tariffs': { to: '/ekorm/bills', keepRest: false },
  tariffs: { to: '/ekorm/tariffs', keepRest: true },
  alarms: { to: '/ekorm/alarms', keepRest: true },
  messages: { to: '/ekorm/messages', keepRest: true },
  reports: { to: '/ekorm/reports', keepRest: true },
  settings: { to: '/ekorm/settings', keepRest: false },
  'iso-50001': { to: '/ekorm/iso-50001', keepRest: true },
  calendar: { to: '/ekorm/calendar', keepRest: true },
  'carbon-footprint': { to: '/ekocm', keepRest: false },
};

const CARBON_PAGE = '/ekorm/carbon-footprint';
const under = (path: string, root: string) => path === root || path.startsWith(`${root}/`);
const withQuery = (path: string, params: URLSearchParams) => (params.size ? `${path}?${params}` : path);

/** R493: where a moved module URL now lives, or null. Callers answer with a permanent (308) redirect. */
export function moduleRedirect(pathname: string, search: string): string | null {
  const params = new URLSearchParams(search);
  if (under(pathname, CARBON_PAGE)) {
    const tab = params.get('tab');
    params.delete('tab');
    return withQuery(sectionHref(tab), params);
  }
  if (under(pathname, '/ekocm/auth')) return `/auth/login?next=${encodeURIComponent('/ekocm')}`;
  const [, segment, ...rest] = pathname.split('/');
  const legacy = Object.hasOwn(LEGACY_ROOTS, segment) ? LEGACY_ROOTS[segment] : undefined;
  if (!legacy) return null;
  const tail = legacy.keepRest && rest.length ? `/${rest.join('/')}` : '';
  return withQuery(`${legacy.to}${tail}`, params);
}
