'use client';

import { useTranslations } from 'next-intl';

import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import type { Option } from '@/components/ui/field';
import { MonthPicker } from '@/components/ui/month-picker';
import { MultiSelect } from '@/components/ui/multi-select';
import { RadioGroup } from '@/components/ui/radio-group';
import { Select } from '@/components/ui/select';

export type PlantSelection = 'all' | 'grid' | 'rooftop';
export type ReportKind = 'monthly' | 'yearly';

export type ReportSelection = {
  buildingIds: string[];
  /** 'YYYY-MM' for a monthly report, 'YYYY' for a yearly one. */
  period: string | null;
  plantSelection: PlantSelection;
  plantIds: string[];
};

export type PlantOption = Option & { kind: 'grid' | 'rooftop' };

export type ReportSelectionViewProps = {
  kind: ReportKind;
  buildings: Option[];
  /** null: this principal may not list plants (GET /power-plants is A CA CR). */
  plants: PlantOption[] | null;
  value: ReportSelection;
  onChange: (value: ReportSelection) => void;
  /** The yearly tab's choices, newest first. */
  years?: number[];
};

/**
 * §7.14's selection: buildings with select-all, the period, and the plant
 * selection with its counter. At least one building stays selected, so a
 * report is never asked of nothing.
 */
export function ReportSelectionView({ kind, buildings, plants, value, onChange, years = [] }: ReportSelectionViewProps) {
  const t = useTranslations('reports.selection');
  const set = (patch: Partial<ReportSelection>) => onChange({ ...value, ...patch });
  const visiblePlants = (plants ?? []).filter((p) => value.plantSelection === 'all' || p.kind === value.plantSelection);

  return (
    <Card className="flex flex-col gap-4">
      <h2 className="type-h3">{t('title')}</h2>
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        <div className="flex flex-col gap-2">
          <MultiSelect
            label={t('buildings')}
            description={t('buildingsHint')}
            options={buildings}
            value={value.buildingIds}
            onValueChange={(ids) => {
              if (ids.length > 0) set({ buildingIds: ids });
            }}
            searchPlaceholder={t('searchBuildings')}
            emptyText={t('noBuildings')}
          />
          <Button
            variant="secondary"
            size="sm"
            className="self-start"
            disabled={buildings.length === 0 || value.buildingIds.length === buildings.length}
            onClick={() => set({ buildingIds: buildings.map((b) => b.value) })}
          >
            {t('selectAll')}
          </Button>
        </div>
        {kind === 'monthly' ? (
          <MonthPicker label={t('month')} value={value.period} onValueChange={(period) => set({ period })} />
        ) : (
          <Select
            label={t('year')}
            options={years.map((y) => ({ value: String(y), label: String(y) }))}
            value={value.period}
            onValueChange={(period) => set({ period })}
          />
        )}
        <div className="flex flex-col gap-2">
          <RadioGroup
            label={t('plantSelection')}
            options={[
              { value: 'all', label: t('plantAll') },
              { value: 'rooftop', label: t('plantRooftop') },
              { value: 'grid', label: t('plantGrid') },
            ]}
            value={value.plantSelection}
            onValueChange={(v) => set({ plantSelection: v as PlantSelection, plantIds: [] })}
          />
          {value.plantSelection !== 'grid' ? <p className="text-foreground-muted type-small">{t('rooftopIncluded')}</p> : null}
        </div>
        {plants !== null && value.plantSelection !== 'rooftop' ? (
          <div className="flex flex-col gap-1">
            <MultiSelect
              label={t('plants')}
              options={visiblePlants}
              value={value.plantIds}
              onValueChange={(plantIds) => set({ plantIds })}
              searchPlaceholder={t('searchPlants')}
              emptyText={t('noPlants')}
            />
            <p className="text-foreground-muted type-small" aria-live="polite">
              {t('plantCount', { count: value.plantIds.length })}
            </p>
          </div>
        ) : null}
      </div>
    </Card>
  );
}
