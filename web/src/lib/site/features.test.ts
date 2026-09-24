import { describe, expect, it } from 'vitest';

import { analyticsScript, siteFeatures } from './features';

describe('siteFeatures', () => {
  it('defaults every flag off, like the Go config', () => {
    expect(siteFeatures({})).toEqual({ pricing: false, analytics: false });
  });

  it('parses booleans the way strconv.ParseBool does', () => {
    for (const v of ['1', 't', 'T', 'true', 'TRUE', 'True']) expect(siteFeatures({ EKOKOD_FEATURE_PRICING_PAGE: v }).pricing).toBe(true);
    for (const v of ['0', 'false', 'yes', '']) expect(siteFeatures({ EKOKOD_FEATURE_PRICING_PAGE: v }).pricing).toBe(false);
  });
});

describe('analyticsScript (Q-H9)', () => {
  const on = { EKOKOD_FEATURE_ANALYTICS: 'true', EKOKOD_ANALYTICS_SRC: 'https://stats.example.com/script.js', EKOKOD_ANALYTICS_SITE: 'ekokod.com' };

  it('needs the flag, an https script and a site id', () => {
    expect(analyticsScript(on)).toEqual({ src: 'https://stats.example.com/script.js', site: 'ekokod.com' });
    expect(analyticsScript({ ...on, EKOKOD_FEATURE_ANALYTICS: 'false' })).toBeNull();
    expect(analyticsScript({ ...on, EKOKOD_ANALYTICS_SRC: 'http://stats.example.com/script.js' })).toBeNull();
    expect(analyticsScript({ ...on, EKOKOD_ANALYTICS_SITE: '' })).toBeNull();
  });
});
