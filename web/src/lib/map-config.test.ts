import { describe, expect, it } from 'vitest';

import { GOOGLE_TILES, OPENFREEMAP_STYLE, fallbackStyleFor, mapOrigins, mapTileUrl } from './map-config';

describe('mapTileUrl', () => {
  it('defaults to Google street tiles with Turkish labels, falling back to OpenFreeMap (F24)', () => {
    expect(mapTileUrl({})).toBe(GOOGLE_TILES);
    expect(GOOGLE_TILES.split(' ')).toHaveLength(4);
    expect(GOOGLE_TILES).toContain('https://mt0.google.com/vt/lyrs=m&hl=tr&x={x}&y={y}&z={z}&scale=2');
    expect(fallbackStyleFor(GOOGLE_TILES)).toBe(OPENFREEMAP_STYLE);
  });

  it('takes the configured source, the runtime variable first, with no fallback', () => {
    expect(mapTileUrl({ EKOKOD_MAP_TILE_URL: 'https://tiles.example.com/{z}/{x}/{y}.png' })).toBe('https://tiles.example.com/{z}/{x}/{y}.png');
    expect(mapTileUrl({ NEXT_PUBLIC_MAP_TILE_URL: 'https://old.example.com/style.json' })).toBe('https://old.example.com/style.json');
    expect(mapTileUrl({ EKOKOD_MAP_TILE_URL: 'OpenFreeMap' })).toBe(OPENFREEMAP_STYLE);
    expect(fallbackStyleFor('https://tiles.example.com/{z}/{x}/{y}.png')).toBe('');
  });

  it('is off for an air-gapped install', () => {
    expect(mapTileUrl({ EKOKOD_MAP_TILE_URL: 'none' })).toBe('');
    expect(mapOrigins({ EKOKOD_MAP_TILE_URL: 'none' })).toEqual([]);
  });

  it('lists every origin the default may load from', () => {
    expect(mapOrigins({})).toEqual([
      'https://mt0.google.com', 'https://mt1.google.com', 'https://mt2.google.com', 'https://mt3.google.com', 'https://tiles.openfreemap.org',
    ]);
    expect(mapOrigins({ EKOKOD_MAP_TILE_URL: 'https://tiles.example.com/{z}/{x}/{y}.png' })).toEqual(['https://tiles.example.com']);
  });
});
