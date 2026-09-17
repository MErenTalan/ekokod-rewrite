import { describe, expect, it, vi } from 'vitest';

import type { Analyzer, Building } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { ALL, AnalyzersTabView } from './analyzers-tab';

const analyzers = [
  {
    id: 'a-1',
    building_id: 'b-1',
    installation_number: '4001234567',
    customer_name: 'Merkez Ofis',
    meter_number: '56529',
    meter_model: 'PM5340',
    meter_multiplier: '1',
    province: 'Ankara',
    district: 'Çankaya',
    tariff_type: 'Sanayi',
    installed_power_kw: '250.5',
    is_active: true,
    provider: 'osos',
    provider_subtype: 'Baskent',
    activity_status: 'active',
    last_reading_at: '2026-09-16T23:00:00+03:00',
  },
] as Analyzer[];
const buildings = [{ id: 'b-1', name: 'A1 Fabrika' }] as Building[];

const view = (overrides: Partial<React.ComponentProps<typeof AnalyzersTabView>> = {}) => (
  <AnalyzersTabView
    analyzers={analyzers}
    buildings={buildings}
    providers={['osos']}
    filters={{ q: '', buildingId: ALL, provider: ALL }}
    onFiltersChange={() => {}}
    canEdit
    canRefresh
    onAssign={() => {}}
    onRefresh={() => {}}
    {...overrides}
  />
);

describe('AnalyzersTabView', () => {
  it('shows every column 01 §7.15 lists', () => {
    const r = renderWithProviders(view());
    for (const header of [
      'Tesisat no',
      'Müşteri adı',
      'Sayaç no',
      'Sayaç modeli',
      'Çarpan',
      'İl / İlçe',
      'Tarife tipi',
      'Kurulu güç (kW)',
      'Bina',
      'Son veri',
      'Durum',
    ]) {
      expect(r.getAllByRole('columnheader', { name: new RegExp(header.replace(/[()/]/g, '.')) }).length, header).toBeGreaterThan(0);
    }
    expect(r.getByRole('row', { name: /4001234567/ })).toHaveTextContent('A1 Fabrika');
  });

  it('lets a building admin refresh but not reassign (R191)', async () => {
    const onRefresh = vi.fn();
    const r = renderWithProviders(view({ canEdit: false, onRefresh }));
    await r.user.click(r.getByRole('button', { name: /İşlemler/ }));
    expect(r.queryByRole('menuitem', { name: 'Binaya ata' })).toBeNull();
    await r.user.click(await r.findByRole('menuitem', { name: 'Saatlik değerleri yenile' }));
    expect(onRefresh).toHaveBeenCalledWith(analyzers[0], 'hourly');
  });

  it('offers no actions at all to a read-only role', () => {
    const r = renderWithProviders(view({ canEdit: false, canRefresh: false }));
    expect(r.queryByRole('button', { name: /İşlemler/ })).toBeNull();
  });

  it('reports a filter change', async () => {
    const onFiltersChange = vi.fn();
    const r = renderWithProviders(view({ onFiltersChange }));
    await r.user.click(r.getByRole('combobox', { name: 'Bina' }));
    await r.user.click(await r.findByRole('option', { name: 'Binaya atanmamış' }));
    expect(onFiltersChange).toHaveBeenCalledWith({ q: '', buildingId: 'unassigned', provider: ALL });
  });
});
