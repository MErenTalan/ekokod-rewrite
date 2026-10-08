'use client';

import { ChevronDown, MapPin } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useMemo, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { EmptyState } from '@/components/ui/empty-state';
import { IconButton } from '@/components/ui/icon-button';
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
  /** Analyzers whose markers the map's analyzer tab shows: checkboxes too, all by default (legacy, F24). */
  visibleAnalyzerIds: string[];
  onVisibleAnalyzersChange: (ids: string[]) => void;
  onSelectBuilding: (id: string) => void;
  onSelectAnalyzer: (id: string) => void;
  onFocus: (id: string) => void;
  loading?: boolean;
};

/**
 * The searchable building list of 01 §7.2: "Seç" drives every other widget,
 * the checkboxes decide which markers the map draws. A building's analyzers
 * unfold under it with the same pair — a checkbox for the map and "Seç" for
 * the panels — instead of a single-choice radio that read as a map filter (F24).
 */
export function BuildingListView({
  buildings,
  analyzers,
  selectedBuildingId,
  selectedAnalyzerId,
  visibleIds,
  onVisibleChange,
  visibleAnalyzerIds,
  onVisibleAnalyzersChange,
  onSelectBuilding,
  onSelectAnalyzer,
  onFocus,
  loading = false,
}: BuildingListViewProps) {
  const t = useTranslations('dashboard.list');
  const [query, setQuery] = useState('');
  const [activeOnly, setActiveOnly] = useState(false);
  const [expanded, setExpanded] = useState<string[]>([]);

  const shown = useMemo(() => {
    const needle = normaliseTurkish(query);
    return buildings.filter((b) => {
      if (activeOnly && b.status !== 'active') return false;
      if (!needle) return true;
      return normaliseTurkish(`${b.name} ${b.address ?? ''}`).includes(needle);
    });
  }, [buildings, query, activeOnly]);

  const shownAnalyzers = analyzers.filter((a) => shown.some((b) => b.id === a.buildingId)).map((a) => a.id);
  const allShown =
    shown.length > 0 && shown.every((b) => visibleIds.includes(b.id)) && shownAnalyzers.every((id) => visibleAnalyzerIds.includes(id));
  const toggleAll = () => {
    onVisibleChange(allShown ? [] : shown.map((b) => b.id));
    onVisibleAnalyzersChange(allShown ? [] : shownAnalyzers);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <p className="text-foreground-muted type-caption">{t('hint')}</p>
        <SearchInput label={t('search')} labelVisibility="hidden" value={query} onValueChange={setQuery} placeholder={t('search')} />
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Checkbox label={t('activeOnly')} checked={activeOnly} onCheckedChange={setActiveOnly} />
          <Button
            size="sm"
            variant="ghost"
            onClick={toggleAll}
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
              // The selected building opens on its analyzers; any other unfolds on demand.
              const open = own.length > 0 && (expanded.includes(building.id) !== selected);
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
                        {own.length > 0 ? (
                          <Button
                            size="sm"
                            variant="ghost"
                            iconEnd={ChevronDown}
                            aria-expanded={open}
                            className="h-auto px-1 py-0 text-foreground-muted type-caption [&[aria-expanded=true]_svg]:rotate-180"
                            onClick={() =>
                              setExpanded(expanded.includes(building.id) ? expanded.filter((id) => id !== building.id) : [...expanded, building.id])
                            }
                          >
                            {t('analyzerCount', { count: building.analyzerCount })}
                          </Button>
                        ) : (
                          <span className="text-foreground-muted type-caption">
                            {t('analyzerCount', { count: building.analyzerCount })}
                          </span>
                        )}
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
                  {open ? (
                    <ul className="flex flex-col gap-1 ps-6" aria-label={t('analyzersOf', { name: building.name || t('unnamed') })}>
                      {own.map((analyzer) => {
                        const chosen = analyzer.id === selectedAnalyzerId;
                        return (
                          <li key={analyzer.id} className="flex items-center justify-between gap-2">
                            <Checkbox
                              label={analyzer.name}
                              checked={visibleAnalyzerIds.includes(analyzer.id)}
                              onCheckedChange={(checked) =>
                                onVisibleAnalyzersChange(
                                  checked
                                    ? [...visibleAnalyzerIds, analyzer.id]
                                    : visibleAnalyzerIds.filter((id) => id !== analyzer.id),
                                )
                              }
                            />
                            <Button
                              size="sm"
                              variant={chosen ? 'primary' : 'ghost'}
                              aria-pressed={chosen}
                              aria-label={t('selectAnalyzer', { name: analyzer.name })}
                              onClick={() => {
                                if (!selected) onSelectBuilding(building.id);
                                onSelectAnalyzer(analyzer.id);
                              }}
                            >
                              {t('select')}
                            </Button>
                          </li>
                        );
                      })}
                    </ul>
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
