type Env = Record<string, string | undefined>;

// Mirrors strconv.ParseBool's true set, so the web and the API read one .env the same way.
const TRUE = new Set(['1', 't', 'T', 'true', 'TRUE', 'True']);
const flag = (v: string | undefined) => TRUE.has(v ?? '');

/** The site's flags, read from the same EKOKOD_FEATURE_* variables as the Go config. */
export function siteFeatures(env: Env = process.env) {
  return { pricing: flag(env.EKOKOD_FEATURE_PRICING_PAGE), analytics: flag(env.EKOKOD_FEATURE_ANALYTICS) };
}

/**
 * Q-H9: no vendor ships. With the flag on, an operator points EKOKOD_ANALYTICS_SRC at a
 * self-hosted, cookie-less script (Plausible/Umami style) and names the site; on-prem leaves it off.
 */
export function analyticsScript(env: Env = process.env): { src: string; site: string } | null {
  if (!siteFeatures(env).analytics) return null;
  const src = env.EKOKOD_ANALYTICS_SRC ?? '';
  const site = env.EKOKOD_ANALYTICS_SITE ?? '';
  if (!src.startsWith('https://') || !site) return null;
  return { src, site };
}
