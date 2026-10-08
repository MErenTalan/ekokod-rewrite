'use client';

import 'maplibre-gl/dist/maplibre-gl.css';

import { List } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useEffect, useRef, useState } from 'react';

import { DEFAULT_MAP_STYLE, DEFAULT_MAP_STYLE_DARK } from '@/lib/map-config';

import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { MapMarkerList } from './_map-marker-list';
import { useMapTileUrl } from './map-config';

export type MapMarker = { id: string; name: string; lat: number; lng: number; status: 'active' | 'passive' };
export type MapProps = {
  markers: MapMarker[];
  label: string;
  /** Marker to centre on and highlight; the list fallback marks the same row (R204). */
  focusId?: string;
  /** Style JSON URL or raster tile template; defaults to the server's EKOKOD_MAP_TILE_URL (F23, plan D23). */
  tileUrl?: string;
  height?: number;
  onMarkerSelect?: (id: string) => void;
};

const TILE_TIMEOUT_MS = 8000;
/** Türkiye, for a map with nothing located on it yet. */
const HOME: { center: [number, number]; zoom: number } = { center: [35.2, 39.0], zoom: 4.6 };

function prefersDark() {
  const theme = document.documentElement.dataset.theme;
  return theme === 'dark' || (theme !== 'light' && window.matchMedia?.('(prefers-color-scheme: dark)').matches === true);
}

function webglAvailable() {
  try {
    const canvas = document.createElement('canvas');
    return Boolean(canvas.getContext('webgl2') ?? canvas.getContext('webgl'));
  } catch {
    return false;
  }
}

/** A `{z}/{x}/{y}` template is raster tiles; any other URL is a style (OpenFreeMap's has no `.json`). */
export const styleFor = (url: string) =>
  url.includes('{z}')
    ? { version: 8 as const, sources: { tiles: { type: 'raster' as const, tiles: [url], tileSize: 256 } }, layers: [{ id: 'tiles', type: 'raster' as const, source: 'tiles' }] }
    : url;

/** MapLibre when a tile source is configured and loads within 8 s; otherwise the coordinate list (no public OSM default, Q5). */
export function Map({ markers, label, focusId, tileUrl: tileUrlProp, height = 360, onMarkerSelect }: MapProps) {
  const t = useTranslations('map');
  const configured = useMapTileUrl();
  const tileUrl = tileUrlProp ?? configured;
  const container = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<'loading' | 'ready' | 'fallback'>(tileUrl ? 'loading' : 'fallback');
  const [showList, setShowList] = useState(false);

  type Instance = import('maplibre-gl').Map;
  type Lib = typeof import('maplibre-gl');
  const instanceRef = useRef<{ map: Instance; lib: Lib } | null>(null);
  // The parent rebuilds its marker array and callback on every render; the map
  // must not: recreating it aborted the style load each time (F23).
  const markersKey = JSON.stringify(markers.map((m) => [m.id, m.name, m.lat, m.lng, m.status]));
  const markersRef = useRef(markers);
  const selectRef = useRef(onMarkerSelect);
  useEffect(() => {
    markersRef.current = markers;
    selectRef.current = onMarkerSelect;
  });

  useEffect(() => {
    if (!tileUrl) return;
    if (!webglAvailable()) {
      setState('fallback');
      return;
    }
    setState('loading');
    let cancelled = false;
    let loaded = false;
    let map: Instance | undefined;
    const timer = setTimeout(() => setState((s) => (s === 'loading' ? 'fallback' : s)), TILE_TIMEOUT_MS);
    import('maplibre-gl')
      .then((maplibre) => {
        if (cancelled || !container.current) return;
        // The bundle loses the worker's own URL; scripts/copy-maplibre-worker.ts serves it (F23).
        maplibre.setWorkerUrl?.(`/maplibre/${maplibre.getVersion?.()}/maplibre-gl-worker.js`);
        const style = tileUrl === DEFAULT_MAP_STYLE && prefersDark() ? DEFAULT_MAP_STYLE_DARK : tileUrl;
        const initial = markersRef.current;
        const bounds = new maplibre.LngLatBounds();
        initial.forEach((m) => bounds.extend([m.lng, m.lat]));
        const instance = new maplibre.Map({
          container: container.current,
          style: styleFor(style),
          ...(initial.length ? { bounds, fitBoundsOptions: { padding: 48, maxZoom: 14 } } : HOME),
          attributionControl: { compact: true },
        });
        map = instance;
        instance.on('load', () => {
          loaded = true;
          clearTimeout(timer);
          if (cancelled) return;
          instanceRef.current = { map: instance, lib: maplibre };
          setState('ready');
        });
        // Only a style that never loads means no map: a single tile or glyph
        // failing later used to swap a working map for the list.
        instance.on('error', () => !cancelled && !loaded && setState('fallback'));
      })
      .catch(() => !cancelled && setState('fallback'));
    return () => {
      cancelled = true;
      clearTimeout(timer);
      instanceRef.current = null;
      map?.remove();
    };
  }, [tileUrl]);

  // Markers follow the data, not the parent's renders: redrawn only when their content changes.
  const fitted = useRef<string | null>(null);
  useEffect(() => {
    const current = instanceRef.current;
    if (state !== 'ready' || !current) return;
    const { map, lib } = current;
    const placed = markersRef.current.map((m) => {
      const el = document.createElement('button');
      el.type = 'button';
      el.setAttribute('aria-label', t('markerLabel', { name: m.name, status: t(m.status) }));
      // Status is shape as well as colour: filled for active, hollow ring for passive.
      el.className =
        m.status === 'active'
          ? 'size-4 rounded-full border-2 border-surface bg-success shadow-md pointer-coarse:size-11'
          : 'size-4 rounded-full border-2 border-foreground-subtle bg-surface pointer-coarse:size-11';
      el.addEventListener('click', () => selectRef.current?.(m.id));
      return new lib.Marker({ element: el }).setLngLat([m.lng, m.lat]).addTo(map);
    });
    if (fitted.current !== null && fitted.current !== markersKey && markersRef.current.length) {
      const bounds = new lib.LngLatBounds();
      markersRef.current.forEach((m) => bounds.extend([m.lng, m.lat]));
      map.fitBounds(bounds, { padding: 48, maxZoom: 14 });
    }
    fitted.current = markersKey;
    return () => placed.forEach((marker) => marker.remove());
  }, [state, markersKey, t]);

  useEffect(() => {
    const marker = markersRef.current.find((m) => m.id === focusId);
    if (state !== 'ready' || !marker) return;
    instanceRef.current?.map.flyTo({ center: [marker.lng, marker.lat], zoom: 14 });
  }, [focusId, markersKey, state]);

  if (state === 'fallback') {
    return (
      <div className="flex flex-col gap-3">
        <Alert tone="info" title={t('tilesUnavailable')} />
        <MapMarkerList markers={markers} label={label} focusId={focusId} onMarkerSelect={onMarkerSelect} />
      </div>
    );
  }
  return (
    <div className="flex flex-col gap-3">
      <div ref={container} role="region" aria-label={label} className="w-full overflow-hidden rounded-lg border border-border bg-surface-sunken" style={{ height }} />
      <Button variant="ghost" size="sm" iconStart={List} className="self-start" aria-expanded={showList} onClick={() => setShowList((s) => !s)}>
        {showList ? t('hideList') : t('showList')}
      </Button>
      {showList ? <MapMarkerList markers={markers} label={label} focusId={focusId} onMarkerSelect={onMarkerSelect} /> : null}
    </div>
  );
}
