import { describe, expect, it } from 'vitest';

import { DEFAULT_MAP_STYLE, mapTileUrl } from './map-config';

describe('mapTileUrl', () => {
  it('defaults to OpenFreeMap when nothing is configured', () => {
    expect(mapTileUrl({})).toBe(DEFAULT_MAP_STYLE);
    expect(mapTileUrl({ EKOKOD_MAP_TILE_URL: '' })).toBe(DEFAULT_MAP_STYLE);
  });

  it('takes the configured source, the runtime variable first', () => {
    expect(mapTileUrl({ EKOKOD_MAP_TILE_URL: 'https://tiles.example.com/{z}/{x}/{y}.png' })).toBe('https://tiles.example.com/{z}/{x}/{y}.png');
    expect(mapTileUrl({ NEXT_PUBLIC_MAP_TILE_URL: 'https://old.example.com/style.json' })).toBe('https://old.example.com/style.json');
  });

  it('is off for an air-gapped install', () => {
    expect(mapTileUrl({ EKOKOD_MAP_TILE_URL: 'none' })).toBe('');
    expect(mapTileUrl({ EKOKOD_MAP_TILE_URL: 'OFF' })).toBe('');
  });
});
