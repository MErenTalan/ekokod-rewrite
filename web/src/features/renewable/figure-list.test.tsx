import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { FigureList, reasonKey } from './figure-list';

describe('FigureList', () => {
  it('shows a value with its unit', () => {
    const r = renderWithProviders(
      <FigureList
        figures={[{ field: 'current_power_kw', value: '12.5', unit: 'kW' }]}
        unavailable={{}}
      />,
    );
    expect(r.getByText('Anlık güç')).toBeVisible();
    expect(r.getByText('12,5 kW')).toBeVisible();
  });

  it('says "veri yok" with the reason sentence, never zero (R165)', () => {
    const r = renderWithProviders(
      <FigureList
        figures={[{ field: 'voltage_v', value: undefined, unit: 'V' }]}
        unavailable={{ voltage_v: 'no_power_quality_measurement' }}
      />,
    );
    expect(r.getByText('veri yok')).toBeVisible();
    expect(r.getByText('Gerilim ve frekans ölçülmüyor.')).toBeVisible();
    expect(r.queryByText(/^0/)).toBeNull();
  });

  it('a missing value without a reason still says "veri yok"', () => {
    const r = renderWithProviders(
      <FigureList figures={[{ field: 'today_kwh', value: null, unit: 'kWh' }]} unavailable={{}} />,
    );
    expect(r.getByText('veri yok')).toBeVisible();
  });

  it('translates closed values and formats money in its currency', () => {
    const r = renderWithProviders(
      <FigureList
        figures={[
          { field: 'status', value: 'producing', format: 'text' },
          { field: 'net_today', value: '-12.3', format: 'money', currency: 'TRY' },
          { field: 'import_price', value: '2.5', format: 'money', currency: 'EUR' },
        ]}
        unavailable={{}}
      />,
    );
    expect(r.getByText('Üretiyor')).toBeVisible();
    expect(r.getByText(/-12,30\s?₺|−12,30\s?₺|₺-12,30|-₺12,30/)).toBeVisible();
    expect(r.getByText('2,5 EUR')).toBeVisible();
  });

  it('maps snake_case reason codes to camelCase message keys', () => {
    expect(reasonKey('no_irradiance_or_capacity_measurement')).toBe(
      'noIrradianceOrCapacityMeasurement',
    );
    expect(reasonKey('consumption_next_24h_kwh')).toBe('consumptionNext24hKwh');
  });
});
