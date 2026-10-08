/**
 * The map's tile source, read by the web server at request time (F23): a
 * NEXT_PUBLIC_ value is baked in at build time, and the Docker build never had
 * one, so the dashboard map always fell back to the coordinate list.
 *
 * `EKOKOD_MAP_TILE_URL` takes a MapLibre style JSON or a raster template;
 * `none` turns the map off (air-gapped installs). Unset means OpenFreeMap:
 * free, keyless, commercial use allowed — not OSM's own tile servers (Q5).
 */
type Env = Record<string, string | undefined>;

export const DEFAULT_MAP_STYLE = 'https://tiles.openfreemap.org/styles/positron';
export const DEFAULT_MAP_STYLE_DARK = 'https://tiles.openfreemap.org/styles/dark';

export function mapTileUrl(env: Env = process.env): string {
  const value = (env.EKOKOD_MAP_TILE_URL || env.NEXT_PUBLIC_MAP_TILE_URL || '').trim();
  if (['none', 'off', 'false'].includes(value.toLowerCase())) return '';
  return value || DEFAULT_MAP_STYLE;
}
