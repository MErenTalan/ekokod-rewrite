import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { MapPanelView, type MapEntity } from './map-panel';

const buildings: MapEntity[] = [
  { id: 'b-1', name: 'A1 Fabrika', lat: '39.92', lng: '32.86', status: 'active' },
  { id: 'b-2', name: 'A2 Depo', lat: '41.00', lng: '28.97', status: 'active' },
  { id: 'b-3', name: 'Şantiye', lat: null, lng: null, status: 'passive' },
];
const analyzers: MapEntity[] = [{ id: 'a-1', name: '4001234567', lat: '39.92', lng: '32.86', status: 'active' }];

describe('MapPanelView', () => {
  it('counts active and passive records and admits the ones without coordinates', () => {
    const r = renderWithProviders(
      <MapPanelView buildings={buildings} analyzers={analyzers} onSelectBuilding={() => {}} onSelectAnalyzer={() => {}} />,
    );
    expect(r.container.querySelector('[data-map-counts]')).toHaveTextContent('Aktif 2 · Pasif 1');
    expect(r.getByText('1 kaydın konumu girilmemiş')).toBeInTheDocument();
    expect(r.queryByRole('row', { name: /Şantiye/ })).toBeNull();
  });

  it('switches to the analyzer map and reports a selection', async () => {
    const onSelectAnalyzer = vi.fn();
    const r = renderWithProviders(
      <MapPanelView buildings={buildings} analyzers={analyzers} onSelectBuilding={() => {}} onSelectAnalyzer={onSelectAnalyzer} />,
    );
    await r.user.click(r.getByRole('tab', { name: 'Analizörler' }));
    expect(r.container.querySelector('[data-map-counts]')).toHaveTextContent('Aktif 1 · Pasif 0');
    await r.user.click(r.getByRole('button', { name: /4001234567/ }));
    expect(onSelectAnalyzer).toHaveBeenCalledWith('a-1');
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(
      <MapPanelView buildings={buildings} analyzers={analyzers} onSelectBuilding={() => {}} onSelectAnalyzer={() => {}} />,
    );
    await expectNoAxeViolations(r.container);
  });
});
