'use client';

import { MapPin } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useMemo, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { EmptyState } from '@/components/ui/empty-state';
import { IconButton } from '@/components/ui/icon-button';
import { RadioGroup } from '@/components/ui/radio-group';
import { SearchInput } from '@/components/ui/search-input';
import { Skeleton } from '@/components/ui/skeleton';
import { StatusBadge } from '@/components/ui/status-badge';
import { normaliseTurkish } from '@/components/ui/combobox';

export type ListBuilding = {
  id: string;
  name: string;
  address: string | null;
  analyzerCount: number;
  status: 'active' | 'passive';
};
export type ListAnalyzer = { id: string; buildingId: string | null; name: string };

export type BuildingListViewProps = {
  buildings: ListBuilding[];
  analyzers: ListAnalyzer[];
  selectedBuildingId?: string;
  selectedAnalyzerId?: string;
  /** Buildings whose markers the map shows; the checkboxes drive it (legacy). */
  visibleIds: string[];
  onVisibleChange: (ids: string[]) => void;
  onSelectBuilding: (id: string) => void;
  onSelectAnalyzer: (id: string) => void;
  onFocus: (id: string) => void;
  loading?: boolean;
};

/**
 * The searchable building list of 01 §7.2: selecting a row drives every other
 * widget, the checkboxes decide which markers the map draws, and a building
 * with several analyzers offers them as a radio group.
 */
export function BuildingListView({
  buildings,
  analyzers,
  selectedBuildingId,
  selectedAnalyzerId,
  visibleIds,
  onVisibleChange,
  onSelectBuilding,
  onSelectAnalyzer,
  onFocus,
  loading = false,
}: BuildingListViewProps) {
  const t = useTranslations('dashboard.list');
  const [query, setQuery] = useState('');
  const [activeOnly, setActiveOnly] = useState(false);

  const shown = useMemo(() => {
    const needle = normaliseTurkish(query);
    return buildings.filter((b) => {
      if (activeOnly && b.status !== 'active') return false;
      if (!needle) return true;
      return normaliseTurkish(`${b.name} ${b.address ?? ''}`).includes(needle);
    });
  }, [buildings, query, activeOnly]);

  const allShown = shown.length > 0 && shown.every((b) => visibleIds.includes(b.id));

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <SearchInput label={t('search')} labelVisibility="hidden" value={query} onValueChange={setQuery} placeholder={t('search')} />
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Checkbox label={t('activeOnly')} checked={activeOnly} onCheckedChange={setActiveOnly} />
          <Button
            size="sm"
            variant="ghost"
            onClick={() => onVisibleChange(allShown ? [] : shown.map((b) => b.id))}
            disabled={shown.length === 0}
          >
            {allShown ? t('deselectAll') : t('selectAll')}
          </Button>
        </div>
        {loading ? (
          <Skeleton className="h-64 w-full" />
        ) : buildings.length === 0 ? (
          <EmptyState title={t('empty')} description={t('emptyHint')} />
        ) : shown.length === 0 ? (
          <EmptyState title={t('noResults')} description={t('emptyHint')} />
        ) : (
          <ul className="flex max-h-96 flex-col gap-2 overflow-y-auto">
            {shown.map((building) => {
              const own = analyzers.filter((a) => a.buildingId === building.id);
              const selected = building.id === selectedBuildingId;
              return (
                <li
                  key={building.id}
                  data-selected={selected || undefined}
                  className="flex flex-col gap-2 rounded-md border border-border p-2 data-[selected]:border-primary data-[selected]:bg-primary-subtle"
                >
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex min-w-0 flex-col gap-1">
                      {/* The checkbox's own visible label is the building name (D14): ticking it shows the marker. */}
                      <Checkbox
                        label={building.name || t('unnamed')}
                        checked={visibleIds.includes(building.id)}
                        onCheckedChange={(checked) =>
                          onVisibleChange(
                            checked ? [...visibleIds, building.id] : visibleIds.filter((id) => id !== building.id),
                          )
                        }
                      />
                      <span className="flex flex-wrap items-center gap-2 ps-6">
                        <StatusBadge
                          status={building.status === 'active' ? 'success' : 'neutral'}
                          label={building.status === 'active' ? t('active') : t('passive')}
                        />
                        <span className="text-foreground-muted type-caption">
                          {t('analyzerCount', { count: building.analyzerCount })}
                        </span>
                      </span>
                    </div>
                    <div className="flex shrink-0 items-center gap-1">
                      <Button
                        size="sm"
                        variant={selected ? 'primary' : 'ghost'}
                        aria-pressed={selected}
                        onClick={() => onSelectBuilding(building.id)}
                      >
                        {t('select')}
                      </Button>
                      <IconButton
                        label={t('centerOnMap', { name: building.name })}
                        icon={MapPin}
                        size="sm"
                        onClick={() => onFocus(building.id)}
                      />
                    </div>
                  </div>
                  {selected && own.length > 1 ? (
                    <div className="ps-8">
                      <RadioGroup
                        label={t('chooseAnalyzer')}
                        options={own.map((a) => ({ value: a.id, label: a.name }))}
                        value={selectedAnalyzerId ?? ''}
                        onValueChange={onSelectAnalyzer}
                      />
                    </div>
                  ) : null}
                </li>
              );
            })}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
