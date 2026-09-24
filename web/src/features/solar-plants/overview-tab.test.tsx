import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoDaily, demoMonthly, demoRealtime, demoRevenue } from './_fixture';
import { OverviewTabView } from './overview-tab';

const view = (props: Partial<Parameters<typeof OverviewTabView>[0]> = {}) =>
  renderWithProviders(
    <OverviewTabView realtime={demoRealtime} revenue={demoRevenue} monthDaily={demoDaily} history={demoMonthly} {...props} />,
  );

describe('OverviewTabView', () => {
  it('shows the six summary figures with units (R282)', () => {
    const r = view();
    for (const label of [/Anlık güç/, /Kapasite kullanımı/, /Bugünkü üretim/, /Bu ayki üretim/, /Bu yılki üretim/, /Toplam üretim/]) {
      expect(r.getByText(label)).toBeVisible();
    }
    expect(r.getByText('125,4')).toBeVisible();
    expect(r.getByText('50,2')).toBeVisible();
  });

  it('says "veri yok" for a figure no inverter reports, never 0', () => {
    const r = view({ realtime: { ...demoRealtime, yield_month_kwh: undefined, capacity_utilisation_pct: undefined } });
    expect(r.getAllByText('veri yok').length).toBeGreaterThanOrEqual(2);
  });

  it('marks stale snapshots with their time', () => {
    const r = view({ realtime: { ...demoRealtime, stale: true } });
    expect(r.getByText(/Eski veri/)).toBeVisible();
  });

  it('shows revenue per currency and flags a partial period (R283)', () => {
    const r = view();
    expect(r.getByText(/Günlük gelir/)).toBeVisible();
    expect(r.getByText('1.625,00')).toBeVisible();
    expect(r.getAllByText('Eksik veri').length).toBe(2); // yearly and total
    expect(r.getByText(/01.01.2026 tarihinden beri/)).toBeVisible();
  });

  it('explains missing revenue instead of a zero (no solar tariff)', () => {
    const r = view({ revenue: { available: false, reason: 'no_solar_tariff' } });
    expect(r.getByText(/GES tarifesi tanımlı değil/)).toBeVisible();
  });

  it('draws the two production charts, each with a data table', () => {
    const r = view();
    expect(r.getByRole('heading', { name: /Bu ayın günlük üretimi/ })).toBeVisible();
    expect(r.getByRole('heading', { name: /Üretim geçmişi/ })).toBeVisible();
  });
});
