import { act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { GOOGLE_TILES, OPENFREEMAP_STYLE } from '@/lib/map-config';

import { Map, styleFor, type MapMarker } from './map';
import { MapConfigProvider } from './_map-config';

type Handler = (e?: unknown) => void;
const created = vi.hoisted(() => [] as { options: Record<string, unknown>; handlers: Record<string, Handler[]>; styles: unknown[] }[]);
const workerUrls = vi.hoisted(() => [] as string[]);
vi.mock('maplibre-gl', () => {
  class FakeMap {
    handlers: Record<string, Handler[]> = {};
    styles: unknown[] = [];
    constructor(options: Record<string, unknown>) {
      created.push({ options, handlers: this.handlers, styles: this.styles });
    }
    setStyle(style: unknown) {
      this.styles.push(style);
    }
    on(event: string, handler: Handler) {
      (this.handlers[event] ??= []).push(handler);
    }
    remove() {}
    flyTo() {}
    fitBounds() {}
  }
  class LngLatBounds {
    extend() {}
  }
  class Marker {
    setLngLat() {
      return this;
    }
    addTo() {
      return this;
    }
    remove() {}
  }
  return { Map: FakeMap, LngLatBounds, Marker, getVersion: () => '6.10.0', setWorkerUrl: workerUrls.push.bind(workerUrls) };
});

const markers: MapMarker[] = [{ id: 'm1', name: 'Merkez Bina', lat: 41.01, lng: 28.97, status: 'active' }];
const fire = (event: string, e?: unknown) => act(() => created.at(-1)!.handlers[event]?.forEach((h) => h(e)));

beforeEach(() => {
  created.length = 0;
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({} as never);
});
afterEach(() => vi.restoreAllMocks());

describe('Map with the server-configured source (F23)', () => {
  it('draws the tile source the server configured', async () => {
    renderWithProviders(
      <MapConfigProvider tileUrl="https://tiles.example.com/style.json">
        <Map markers={markers} label="Bina konumları" />
      </MapConfigProvider>,
    );
    await waitFor(() => expect(created).toHaveLength(1));
    expect(created[0]!.options.style).toBe('https://tiles.example.com/style.json');
    // The bundled import.meta.url is a build-machine path; the worker comes from public/ instead.
    expect(workerUrls).toContain('/maplibre/6.10.0/maplibre-gl-worker.js');
  });

  it('keeps a loaded map when a tile fails later, and falls back when the style never loads', async () => {
    const r = renderWithProviders(<Map markers={markers} label="Bina konumları" tileUrl="https://tiles.example.com/style.json" />);
    await waitFor(() => expect(created).toHaveLength(1));
    fire('load');
    fire('error');
    expect(r.queryByRole('table', { name: 'Bina konumları' })).toBeNull();
    expect(r.getByRole('button', { name: 'Konum listesini göster' })).toBeInTheDocument();
    r.unmount();

    const again = renderWithProviders(<Map markers={markers} label="Bina konumları" tileUrl="https://tiles.example.com/style.json" />);
    await waitFor(() => expect(created).toHaveLength(2));
    fire('error');
    expect(again.getByRole('table', { name: 'Bina konumları' })).toBeInTheDocument();
  });

  it('opens on Türkiye when nothing is located yet', async () => {
    renderWithProviders(<Map markers={[]} label="Bina konumları" tileUrl="https://tiles.example.com/style.json" />);
    await waitFor(() => expect(created).toHaveLength(1));
    expect(created[0]!.options).toMatchObject({ center: [35.2, 39.0] });
  });

  it('keeps one map while the parent re-renders with equal markers and a new callback', async () => {
    const r = renderWithProviders(<Map markers={[...markers]} label="Bina konumları" tileUrl="https://tiles.example.com/style.json" onMarkerSelect={() => {}} />);
    await waitFor(() => expect(created).toHaveLength(1));
    fire('load');
    r.rerender(<Map markers={[...markers]} label="Bina konumları" tileUrl="https://tiles.example.com/style.json" onMarkerSelect={() => {}} />);
    r.rerender(<Map markers={markers.map((m) => ({ ...m }))} label="Bina konumları" tileUrl="https://tiles.example.com/style.json" onMarkerSelect={() => {}} />);
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(created).toHaveLength(1);
  });

  it('reads an extensionless style URL as a style and only a {z}/{x}/{y} template as raster', () => {
    expect(styleFor('https://tiles.openfreemap.org/styles/positron')).toBe('https://tiles.openfreemap.org/styles/positron');
    expect(styleFor('https://x.example.com/style.json')).toBe('https://x.example.com/style.json');
    expect(styleFor('https://tile.example.com/{z}/{x}/{y}.png')).toMatchObject({ sources: { tiles: { type: 'raster' } } });
  });

  it('swaps refused Google tiles for OpenFreeMap once, and keeps them once a tile drew', async () => {
    const r = renderWithProviders(<Map markers={markers} label="Bina konumları" tileUrl={GOOGLE_TILES} />);
    await waitFor(() => expect(created).toHaveLength(1));
    expect(created[0]!.options.style).toMatchObject({ sources: { tiles: { tiles: GOOGLE_TILES.split(' '), attribution: '© Google', maxzoom: 20 } } });
    fire('load');
    fire('error', { tile: {}, error: new Error('403') });
    fire('error', { tile: {}, error: new Error('403') });
    expect(created[0]!.styles).toEqual([OPENFREEMAP_STYLE]);
    expect(r.queryByRole('table', { name: 'Bina konumları' })).toBeNull();
    r.unmount();

    renderWithProviders(<Map markers={markers} label="Bina konumları" tileUrl={GOOGLE_TILES} />);
    await waitFor(() => expect(created).toHaveLength(2));
    fire('sourcedata', { tile: {} });
    fire('error', { tile: {}, error: new Error('429') });
    expect(created[1]!.styles).toEqual([]);
  });
});
