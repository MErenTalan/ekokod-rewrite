'use client';

import { useTranslations } from 'next-intl';

import { Checkbox } from '../ui/checkbox';
import { Combobox } from '../ui/combobox';

export type PickerBuilding = { id: string; name: string; active: boolean };
export type PickerAnalyzer = { id: string; buildingId: string; name: string; active: boolean };
export type BuildingAnalyzerValue = { buildingId: string | null; analyzerId: string | null };
export type BuildingAnalyzerPickerProps = {
  buildings: PickerBuilding[];
  analyzers: PickerAnalyzer[];
  value: BuildingAnalyzerValue;
  onValueChange: (value: BuildingAnalyzerValue) => void;
  activeOnly: boolean;
  onActiveOnlyChange: (activeOnly: boolean) => void;
};

/** Building → analyzer cascade (07 §6). Presentational: remembering the selection is the caller's store (F6, plan D17). */
export function BuildingAnalyzerPicker({ buildings, analyzers, value, onValueChange, activeOnly, onActiveOnlyChange }: BuildingAnalyzerPickerProps) {
  const t = useTranslations('domain.picker');
  const visible = <T extends { active: boolean }>(items: T[]) => (activeOnly ? items.filter((i) => i.active) : items);
  const analyzerOptions = visible(analyzers.filter((a) => a.buildingId === value.buildingId));
  return (
    <div className="flex flex-wrap items-end gap-3">
      <div className="w-64 max-w-full">
        <Combobox
          label={t('building')}
          options={visible(buildings).map((b) => ({ value: b.id, label: b.name }))}
          value={value.buildingId}
          searchPlaceholder={t('searchBuilding')}
          emptyText={t('noBuildings')}
          onValueChange={(buildingId) => {
            const keep = analyzers.some((a) => a.id === value.analyzerId && a.buildingId === buildingId);
            onValueChange({ buildingId, analyzerId: keep ? value.analyzerId : null });
          }}
        />
      </div>
      <div className="w-64 max-w-full">
        <Combobox
          label={t('analyzer')}
          options={analyzerOptions.map((a) => ({ value: a.id, label: a.name }))}
          value={value.analyzerId}
          searchPlaceholder={t('searchAnalyzer')}
          emptyText={t('noAnalyzers')}
          disabled={value.buildingId === null}
          // A placeholder, not a description: a line under one field misaligns every filter row (items-end).
          placeholder={value.buildingId === null ? t('chooseBuildingFirst') : undefined}
          onValueChange={(analyzerId) => onValueChange({ ...value, analyzerId })}
        />
      </div>
      <div className="flex h-9 items-center pointer-coarse:min-h-11">
        <Checkbox label={t('activeOnly')} checked={activeOnly} onCheckedChange={onActiveOnlyChange} />
      </div>
    </div>
  );
}
