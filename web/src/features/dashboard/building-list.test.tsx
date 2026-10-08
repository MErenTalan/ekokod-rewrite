import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { BuildingListView, type ListAnalyzer, type ListBuilding } from './building-list';

const buildings: ListBuilding[] = [
  { id: 'b-1', name: 'IĞDIR Deposu', address: 'Iğdır OSB', analyzerCount: 2, status: 'active' },
  { id: 'b-2', name: 'Tuzla Fabrika', address: 'Tuzla, İstanbul', analyzerCount: 1, status: 'passive' },
];
const analyzers: ListAnalyzer[] = [
  { id: 'a-1', buildingId: 'b-1', name: 'Analizör 1' },
  { id: 'a-2', buildingId: 'b-1', name: 'Analizör 2' },
  { id: 'a-3', buildingId: 'b-2', name: 'Analizör 3' },
];

const view = (overrides: Partial<React.ComponentProps<typeof BuildingListView>> = {}) => (
  <BuildingListView
    buildings={buildings}
    analyzers={analyzers}
    visibleIds={['b-1', 'b-2']}
    onVisibleChange={() => {}}
    visibleAnalyzerIds={['a-1', 'a-2', 'a-3']}
    onVisibleAnalyzersChange={() => {}}
    onSelectBuilding={() => {}}
    onSelectAnalyzer={() => {}}
    onFocus={() => {}}
    {...overrides}
  />
);

describe('BuildingListView', () => {
  it('searches case- and diacritic-insensitively in Turkish', async () => {
    const r = renderWithProviders(view());
    await r.user.type(r.getByRole('searchbox', { name: 'Bina veya adres ara' }), 'ığdır');
    expect(r.getByRole('checkbox', { name: 'IĞDIR Deposu' })).toBeInTheDocument();
    expect(r.queryByRole('checkbox', { name: 'Tuzla Fabrika' })).toBeNull();
  });

  it('filters to active buildings and clears the whole visible set, analyzers included', async () => {
    const onVisibleChange = vi.fn();
    const onVisibleAnalyzersChange = vi.fn();
    const r = renderWithProviders(view({ onVisibleChange, onVisibleAnalyzersChange }));
    await r.user.click(r.getByRole('checkbox', { name: 'Yalnızca aktif' }));
    expect(r.queryByRole('checkbox', { name: 'Tuzla Fabrika' })).toBeNull();
    await r.user.click(r.getByRole('button', { name: 'Tümünü kaldır' }));
    expect(onVisibleChange).toHaveBeenCalledWith([]);
    expect(onVisibleAnalyzersChange).toHaveBeenCalledWith([]);
  });

  it('shows analyzers on the map by checkbox, several at once, not by a single choice (F24)', async () => {
    const onVisibleAnalyzersChange = vi.fn();
    const r = renderWithProviders(view({ selectedBuildingId: 'b-1', visibleAnalyzerIds: ['a-3'], onVisibleAnalyzersChange }));
    expect(r.queryByRole('radio')).toBeNull();
    await r.user.click(r.getByRole('checkbox', { name: 'Analizör 1' }));
    expect(onVisibleAnalyzersChange).toHaveBeenLastCalledWith(['a-3', 'a-1']);
    // Another building's analyzers unfold on demand.
    expect(r.queryByRole('checkbox', { name: 'Analizör 3' })).toBeNull();
    await r.user.click(r.getByRole('button', { name: '1 analizör' }));
    expect(r.getByRole('checkbox', { name: 'Analizör 3' })).toBeChecked();
  });

  it('offers the analyzers of the selected building and reports the choice', async () => {
    const onSelectAnalyzer = vi.fn();
    const onSelectBuilding = vi.fn();
    const onFocus = vi.fn();
    const r = renderWithProviders(
      view({ selectedBuildingId: 'b-1', selectedAnalyzerId: 'a-1', onSelectAnalyzer, onSelectBuilding, onFocus }),
    );
    expect(r.getByRole('button', { name: 'Analizör 1 analizörünü seç' })).toHaveAttribute('aria-pressed', 'true');
    await r.user.click(r.getByRole('button', { name: 'Analizör 2 analizörünü seç' }));
    expect(onSelectAnalyzer).toHaveBeenCalledWith('a-2');
    // Every row offers "Seç"; the second one belongs to the building that is not selected yet.
    await r.user.click(r.getAllByRole('button', { name: 'Seç' })[1]);
    expect(onSelectBuilding).toHaveBeenCalledWith('b-2');
    await r.user.click(r.getByRole('button', { name: 'IĞDIR Deposu binasını haritada göster' }));
    expect(onFocus).toHaveBeenCalledWith('b-1');
  });

  it('explains an empty list and a search with no match', async () => {
    const empty = renderWithProviders(view({ buildings: [], analyzers: [] }));
    expect(empty.getByText('Henüz bina tanımlanmamış')).toBeInTheDocument();
    empty.unmount();
    const r = renderWithProviders(view());
    await r.user.type(r.getByRole('searchbox', { name: 'Bina veya adres ara' }), 'zzz');
    expect(r.getByText('Aramanızla eşleşen bina yok')).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(view({ selectedBuildingId: 'b-1' }));
    await expectNoAxeViolations(r.container);
  });
});
