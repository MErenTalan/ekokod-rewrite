'use client';

import 'maplibre-gl/dist/maplibre-gl.css';

import { List } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useEffect, useRef, useState } from 'react';

import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { MapMarkerList } from './_map-marker-list';

export type MapMarker = { id: string; name: string; lat: number; lng: number; status: 'active' | 'passive' };
export type MapProps = {
  markers: MapMarker[];
  label: string;
  /** Marker to centre on and highlight; the list fallback marks the same row (R204). */
  focusId?: string;
  /** Style JSON URL or raster tile template; defaults to NEXT_PUBLIC_MAP_TILE_URL (plan D23). */
  tileUrl?: string;
  height?: number;
  onMarkerSelect?: (id: string) => void;
};

const TILE_TIMEOUT_MS = 8000;

function webglAvailable() {
  try {
    const canvas = document.createElement('canvas');
    return Boolean(canvas.getContext('webgl2') ?? canvas.getContext('webgl'));
  } catch {
    return false;
  }
}

const styleFor = (url: string) =>
  url.endsWith('.json')
    ? url
    : { version: 8 as const, sources: { tiles: { type: 'raster' as const, tiles: [url], tileSize: 256 } }, layers: [{ id: 'tiles', type: 'raster' as const, source: 'tiles' }] };

/** MapLibre when a tile source is configured and loads within 8 s; otherwise the coordinate list (no public OSM default, Q5). */
export function Map({ markers, label, focusId, tileUrl = process.env.NEXT_PUBLIC_MAP_TILE_URL ?? '', height = 360, onMarkerSelect }: MapProps) {
  const t = useTranslations('map');
  const container = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<'loading' | 'ready' | 'fallback'>(tileUrl ? 'loading' : 'fallback');
  const [showList, setShowList] = useState(false);

  const instanceRef = useRef<{ flyTo(options: { center: [number, number]; zoom: number }): void } | null>(null);

  useEffect(() => {
    if (!tileUrl) return;
    if (!webglAvailable()) {
      setState('fallback');
      return;
    }
    let cancelled = false;
    let map: { remove(): void } | undefined;
    const timer = setTimeout(() => setState((s) => (s === 'loading' ? 'fallback' : s)), TILE_TIMEOUT_MS);
    import('maplibre-gl')
      .then((maplibre) => {
        if (cancelled || !container.current) return;
        const bounds = new maplibre.LngLatBounds();
        markers.forEach((m) => bounds.extend([m.lng, m.lat]));
        const instance = new maplibre.Map({ container: container.current, style: styleFor(tileUrl), bounds: markers.length ? bounds : undefined, fitBoundsOptions: { padding: 48, maxZoom: 14 } });
        map = instance;
        instanceRef.current = instance;
        instance.on('load', () => {
          clearTimeout(timer);
          if (!cancelled) setState('ready');
        });
        instance.on('error', () => !cancelled && setState('fallback'));
        for (const m of markers) {
          const el = document.createElement('button');
          el.type = 'button';
          el.setAttribute('aria-label', t('markerLabel', { name: m.name, status: t(m.status) }));
          // Status is shape as well as colour: filled for active, hollow ring for passive.
          el.className =
            m.status === 'active'
              ? 'size-4 rounded-full border-2 border-surface bg-success shadow-md pointer-coarse:size-11'
              : 'size-4 rounded-full border-2 border-foreground-subtle bg-surface pointer-coarse:size-11';
          el.addEventListener('click', () => onMarkerSelect?.(m.id));
          new maplibre.Marker({ element: el }).setLngLat([m.lng, m.lat]).addTo(instance);
        }
      })
      .catch(() => !cancelled && setState('fallback'));
    return () => {
      cancelled = true;
      clearTimeout(timer);
      instanceRef.current = null;
      map?.remove();
    };
  }, [tileUrl, markers, onMarkerSelect, t]);

  useEffect(() => {
    const marker = markers.find((m) => m.id === focusId);
    if (state !== 'ready' || !marker) return;
    instanceRef.current?.flyTo({ center: [marker.lng, marker.lat], zoom: 14 });
  }, [focusId, markers, state]);

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
