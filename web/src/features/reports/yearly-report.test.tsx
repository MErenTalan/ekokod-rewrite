import { screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoYearly } from './_fixture';
import { YearlyReportView } from './yearly-report';

const table = (name: string) => screen.getByRole('table', { name });

describe('YearlyReportView', () => {
  it('prints twelve months, the yearly total and the two daily averages', () => {
    renderWithProviders(<YearlyReportView payload={demoYearly} />);
    const rows = within(table('Elektrik tüketimi ve çatı GES üretimi')).getAllByRole('row');
    expect(rows).toHaveLength(1 + 12 + 3); // header, months, total and two averages
    expect(rows[13]).toHaveTextContent('Yıllık toplam');
    expect(rows[13]).toHaveTextContent('216.000 kWh');
    expect(rows[14]).toHaveTextContent('Günlük ortalama tüketim');
    expect(rows[15]).toHaveTextContent('Çatı GES günlük ortalama üretim');
  });

  it('reports the plants against their targets, and a plant without one plainly', () => {
    renderWithProviders(<YearlyReportView payload={demoYearly} />);
    const solar = within(table('Güneş enerjisi üretim raporu'));
    expect(solar.getByRole('row', { name: /Hedef gerçekleşme oranı/ })).toHaveTextContent('%90');
    expect(solar.getByRole('row', { name: /Kuzey GES/ })).toHaveTextContent('Hedef tanımlı değil');
  });

  it('shows no achievement when there is no target (E-1)', () => {
    renderWithProviders(<YearlyReportView payload={{ ...demoYearly, target: undefined, achievement_pct: undefined }} />);
    const solar = within(table('Güneş enerjisi üretim raporu'));
    expect(solar.getByRole('row', { name: /Hedeflenen üretim/ })).toHaveTextContent('veri yok');
    expect(solar.getByRole('row', { name: /Hedef gerçekleşme oranı/ })).toHaveTextContent('veri yok');
    expect(solar.getByRole('row', { name: /Hedef gerçekleşme oranı/ })).not.toHaveTextContent('%0');
  });

  it('prints the final comparison and the carbon section with its factor', () => {
    renderWithProviders(<YearlyReportView payload={demoYearly} />);
    const comparison = within(table('Nihai karşılaştırma'));
    expect(comparison.getByRole('row', { name: /GES ile karşılanan/ })).toHaveTextContent('%74,8');
    expect(comparison.getByRole('row', { name: /Şebekeden alınan/ })).toHaveTextContent('%25,2');
    const carbon = within(table('Karbon emisyonu'));
    expect(carbon.getByRole('row', { name: /Nihai emisyon/ })).toHaveTextContent('24,48 ton/yıl');
    expect(carbon.getByRole('row', { name: /Kullanılan emisyon faktörü/ })).toHaveTextContent('0,45 kg CO2e/kWh');
    expect(screen.getByText('Kaynak yılı: 2022')).toBeVisible();
    expect(screen.getByText('Faktör Türkiye şebeke ortalamasıdır.')).toBeVisible();
  });

  it('prints a net exporter’s negative net emission with its sign (R262)', () => {
    renderWithProviders(<YearlyReportView payload={{ ...demoYearly, carbon: { ...demoYearly.carbon!, net_t: '-3.24' } }} />);
    expect(within(table('Karbon emisyonu')).getByRole('row', { name: /Nihai emisyon/ })).toHaveTextContent('-3,24 ton/yıl');
  });

  it('says why the carbon section is empty without a factor', () => {
    renderWithProviders(<YearlyReportView payload={{ ...demoYearly, carbon: undefined, carbon_reason: 'grid_factor_missing' }} />);
    expect(screen.getByText(/emisyon faktörü tanımlı değil/)).toBeVisible();
    expect(screen.queryByRole('table', { name: 'Karbon emisyonu' })).toBeNull();
  });

  it('omits the shares when consumption is unknown', () => {
    renderWithProviders(<YearlyReportView payload={{ ...demoYearly, solar_share_pct: undefined, grid_share_pct: undefined }} />);
    expect(within(table('Nihai karşılaştırma')).getByRole('row', { name: /GES ile karşılanan/ })).toHaveTextContent('veri yok');
  });

  it('draws the three charts', () => {
    renderWithProviders(<YearlyReportView payload={demoYearly} />);
    for (const title of ['Yıllık elektrik tüketimi ve üretimi', 'Yıllık elektrik faturası', 'Hedeflenen ve gerçekleşen üretim']) {
      expect(screen.getAllByText(title)[0]).toBeVisible();
    }
  });
});
