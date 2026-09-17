import { describe, expect, it, vi } from 'vitest';

import type { BuildingComparison } from '@/lib/api/types';
import { toCsv } from '@/lib/csv';
import { messages } from '../../../messages';
import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { SectorComparisonView, sectorCsvRows } from './sector-comparison';

const metric = (value: string, average: string, rank: number, ranked: number) => ({ value, average, rank, ranked });
const comparison: BuildingComparison = {
  available: true,
  sector: 'Üretim',
  peers: 4,
  daily_consumption: metric('412.5', '380', 2, 4),
  monthly_consumption: metric('12375', '11400.25', 3, 4),
  co2_emission_kg: metric('5803.87', '5346.71', 3, 4),
  consumption_per_capita: metric('68.75', '61.5', 2, 4),
  consumption_per_area: metric('2.475', '2.28', 0, 0),
};
const t = (key: string, values?: Record<string, string | number>) => {
  const raw = (messages.tr.dashboard.sector as Record<string, string>)[key] ?? key;
  return raw.replace(/\{(\w+)\}/g, (_, name: string) => String(values?.[name] ?? ''));
};

describe('SectorComparisonView', () => {
  it('shows the building against its sector average with the ranks legacy showed', () => {
    const r = renderWithProviders(
      <SectorComparisonView buildingName="A1 Fabrika" comparison={comparison} canEditBuildings onExport={() => {}} />,
    );
    expect(r.getByText('Sektör: Üretim · 4 bina')).toBeInTheDocument();
    expect(r.getByText('412,5')).toBeInTheDocument();
    expect(r.getByText('11.400,25')).toBeInTheDocument();
    expect(r.getByText(/Aylık tüketim \(kWh\/ay\): 4 bina içinde 3\./)).toBeInTheDocument();
    expect(r.getByText(/Birim alan başına \(kWh\/m²·ay\): sıralanamadı/)).toBeInTheDocument();
  });

  it('explains a sector that is too small, and a building without a sector', () => {
    const small = renderWithProviders(
      <SectorComparisonView
        buildingName="A1 Fabrika"
        comparison={{ ...comparison, available: false, reason: 'sector_too_small' }}
        canEditBuildings={false}
        onExport={() => {}}
      />,
    );
    expect(small.getByText('Sektörde karşılaştırma için yeterli bina yok (en az 3).')).toBeInTheDocument();
    small.unmount();

    const missing = renderWithProviders(
      <SectorComparisonView buildingName="A1 Fabrika" comparison={null} sectorMissing canEditBuildings onExport={() => {}} />,
    );
    expect(missing.getByRole('link', { name: 'Binayı düzenle' })).toHaveAttribute('href', '/ekorm/settings?tab=buildings');
  });

  it('hides the edit link from a role that may not edit buildings', () => {
    const r = renderWithProviders(
      <SectorComparisonView buildingName="A1 Fabrika" comparison={null} sectorMissing canEditBuildings={false} onExport={() => {}} />,
    );
    expect(r.queryByRole('link', { name: 'Binayı düzenle' })).toBeNull();
  });

  it('exports the table, the average and the ranks as Turkish CSV', async () => {
    const onExport = vi.fn();
    const r = renderWithProviders(
      <SectorComparisonView buildingName="A1 Fabrika" comparison={comparison} canEditBuildings onExport={onExport} />,
    );
    await r.user.click(r.getByRole('button', { name: 'Dışa aktar' }));
    await r.user.click(await r.findByRole('menuitem', { name: 'CSV' }));
    expect(onExport).toHaveBeenCalled();

    const csv = toCsv(sectorCsvRows(comparison, 'A1 Fabrika', t));
    expect(csv.startsWith('﻿')).toBe(true);
    const [header, building, average, ...ranks] = csv.split('\r\n');
    expect(header.split(';')[1]).toBe('Günlük tüketim (kWh/gün)');
    expect(building.split(';')[0]).toBe('A1 Fabrika');
    expect(building.split(';')[2]).toBe('12.375');
    expect(average.split(';')[0]).toBe('Sektör ortalaması');
    expect(ranks[0]).toContain('4 bina içinde 2.');
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(
      <SectorComparisonView buildingName="A1 Fabrika" comparison={comparison} canEditBuildings onExport={() => {}} />,
    );
    await expectNoAxeViolations(r.container);
  });
});
