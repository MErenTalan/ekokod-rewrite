'use client';

import { useTranslations } from 'next-intl';
import { useMemo, useState } from 'react';

import { Map, type MapMarker } from '@/components/map/map';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs } from '@/components/ui/tabs';

export type MapEntity = {
  id: string;
  name: string;
  lat: string | null;
  lng: string | null;
  status: 'active' | 'passive';
};

export type MapPanelViewProps = {
  buildings: MapEntity[];
  analyzers: MapEntity[];
  focusId?: string;
  onSelectBuilding: (id: string) => void;
  onSelectAnalyzer: (id: string) => void;
  loading?: boolean;
};

const markersOf = (entities: MapEntity[]): MapMarker[] =>
  entities
    .filter((e) => e.lat !== null && e.lng !== null)
    .map((e) => ({ id: e.id, name: e.name, lat: Number(e.lat), lng: Number(e.lng), status: e.status }));

/**
 * The dashboard's building and analyzer maps (01 §7.2): active and passive
 * counts in the header, a marker per located entity, and an honest note for the
 * records that carry no coordinates — real installations often have none.
 */
export function MapPanelView({
  buildings,
  analyzers,
  focusId,
  onSelectBuilding,
  onSelectAnalyzer,
  loading = false,
}: MapPanelViewProps) {
  const t = useTranslations('dashboard.map');
  const [tab, setTab] = useState('buildings');
  const shown = tab === 'analyzers' ? analyzers : buildings;
  const markers = useMemo(() => markersOf(shown), [shown]);
  const active = shown.filter((e) => e.status === 'active').length;
  const missing = shown.length - markers.length;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <p className="text-foreground-muted type-small" data-map-counts>
          {t('counts', { active, passive: shown.length - active })}
        </p>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {loading ? (
          <Skeleton className="h-[360px] w-full" />
        ) : (
          <>
            <Tabs
              value={tab}
              onValueChange={setTab}
              items={[
                {
                  value: 'buildings',
                  label: t('buildings'),
                  content: (
                    <Map markers={markers} label={t('buildingsLabel')} focusId={focusId} onMarkerSelect={onSelectBuilding} />
                  ),
                },
                {
                  value: 'analyzers',
                  label: t('analyzers'),
                  content: (
                    <Map markers={markers} label={t('analyzersLabel')} focusId={focusId} onMarkerSelect={onSelectAnalyzer} />
                  ),
                },
              ]}
            />
            {missing > 0 ? (
              <p className="text-foreground-muted type-caption">{t('missingCoordinates', { count: missing })}</p>
            ) : null}
          </>
        )}
      </CardContent>
    </Card>
  );
}
