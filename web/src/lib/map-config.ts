/**
 * The map's tile source, read by the web server at request time (F23).
 *
 * `EKOKOD_MAP_TILE_URL` takes a MapLibre style URL or raster `{z}/{x}/{y}`
 * templates (several, space-separated, are load-balanced); `openfreemap` picks
 * the OpenFreeMap style, `none` turns the map off (air-gapped installs). Unset
 * means Google's street tiles with Turkish labels, as legacy used (F24): not a
 * documented Google API, so the map falls back to OpenFreeMap when they fail.
 */
type Env = Record<string, string | undefined>;

export const GOOGLE_TILES = [0, 1, 2, 3].map((s) => `https://mt${s}.google.com/vt/lyrs=m&hl=tr&x={x}&y={y}&z={z}&scale=2`).join(' ');
export const OPENFREEMAP_STYLE = 'https://tiles.openfreemap.org/styles/liberty';

export function mapTileUrl(env: Env = process.env): string {
  const value = (env.EKOKOD_MAP_TILE_URL || env.NEXT_PUBLIC_MAP_TILE_URL || '').trim();
  const key = value.toLowerCase();
  if (['none', 'off', 'false'].includes(key)) return '';
  if (key === 'openfreemap') return OPENFREEMAP_STYLE;
  return value || GOOGLE_TILES;
}

/** The style a failing source gives way to: only the unofficial Google default has one. */
export const fallbackStyleFor = (source: string) => (source === GOOGLE_TILES ? OPENFREEMAP_STYLE : '');

/** Every origin the map may load from, for the CSP. */
export function mapOrigins(env: Env = process.env): string[] {
  const source = mapTileUrl(env);
  const urls = [...source.split(/\s+/), fallbackStyleFor(source)].filter(Boolean);
  const origins = urls.flatMap((url) => {
    try {
      const u = new URL(url.replace(/\{[^}]*\}/g, '0'));
      return u.protocol === 'https:' || u.protocol === 'http:' ? [u.origin] : [];
    } catch {
      return [];
    }
  });
  return [...new Set(origins)];
}
