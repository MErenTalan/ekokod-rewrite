'use client';

import { useEffect } from 'react';

import { BuildingAnalyzerPicker, type BuildingAnalyzerPickerProps } from '@/components/domain/building-analyzer-picker';
import { Skeleton } from '@/components/ui/skeleton';
import { useSelection } from '@/lib/selection/selection-store';

import { analyzerLabel, useAssets } from './use-assets';

export type ScopePickerViewProps = BuildingAnalyzerPickerProps & { loading?: boolean };

/** The picker itself: a skeleton the same height while the assets load. */
export function ScopePickerView({ loading = false, ...props }: ScopePickerViewProps) {
  if (loading) return <Skeleton className="h-16 w-full max-w-xl" />;
  return <BuildingAnalyzerPicker {...props} />;
}

export type ScopePickerProps = {
  activeOnly: boolean;
  onActiveOnlyChange: (value: boolean) => void;
};

/**
 * The building → analyzer cascade bound to the remembered selection (R196): the
 * first building is chosen when nothing is stored, a building with exactly one
 * analyzer selects it (legacy BuildingList), and a stored id that is no longer
 * visible is replaced rather than silently kept.
 */
export function ScopePicker({ activeOnly, onActiveOnlyChange }: ScopePickerProps) {
  const { buildings, analyzers, loading } = useAssets();
  const { buildingId, analyzerId, set } = useSelection();

  useEffect(() => {
    if (loading || buildings.length === 0) return;
    const building = buildings.find((b) => b.id === buildingId) ?? buildings[0];
    const own = analyzers.filter((a) => a.building_id === building.id);
    const analyzer = own.find((a) => a.id === analyzerId) ?? (own.length === 1 ? own[0] : undefined);
    if (building.id !== buildingId || analyzer?.id !== analyzerId) {
      set({ buildingId: building.id, analyzerId: analyzer?.id });
    }
  }, [loading, buildings, analyzers, buildingId, analyzerId, set]);

  return (
    <ScopePickerView
      loading={loading}
      buildings={buildings.map((b) => ({ id: b.id, name: b.name, active: b.activity_status !== 'passive' }))}
      analyzers={analyzers.map((a) => ({
        id: a.id,
        buildingId: a.building_id ?? '',
        name: analyzerLabel(a),
        active: a.activity_status === 'active',
      }))}
      value={{ buildingId: buildingId ?? null, analyzerId: analyzerId ?? null }}
      onValueChange={(next) => set({ buildingId: next.buildingId ?? undefined, analyzerId: next.analyzerId ?? undefined })}
      activeOnly={activeOnly}
      onActiveOnlyChange={onActiveOnlyChange}
    />
  );
}
