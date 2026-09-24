import { screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoMonthly } from './_fixture';
import { MonthlyReportView } from './monthly-report';

const rows = [
  'Rapor dönemi', 'Bina(lar)', 'Elektrik tarifesi', 'Elektrik alış fiyatı', 'Ortalama alış fiyatı',
  'Çatı GES satış fiyatı', 'Arazi GES satış fiyatı', 'Aylık toplam tüketim', 'Günlük ortalama tüketim',
  'Çatı GES aylık üretimi', 'Arazi GES aylık üretimi', 'Toplam üretim', 'Günlük ortalama üretim',
  'Elektrik faturası', 'Reaktif ceza',
];

function infoRow(label: string) {
  const table = screen.getByRole('table', { name: 'Bilgi tablosu' });
  const escaped = label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  return within(table).getByRole('row', { name: new RegExp(`^${escaped}`) });
}

describe('MonthlyReportView', () => {
  it('prints every row of §7.14’s information table', () => {
    renderWithProviders(<MonthlyReportView payload={demoMonthly} />);
    for (const label of rows) expect(infoRow(label)).toBeVisible();
    expect(infoRow('Rapor dönemi')).toHaveTextContent('Ağustos 2026');
    expect(infoRow('Elektrik tarifesi')).toHaveTextContent('Merkez: Sanayi OG çift terimli');
    expect(infoRow('Elektrik tarifesi')).toHaveTextContent('Depo: veri yok');
    expect(infoRow('Aylık toplam tüketim')).toHaveTextContent('18.450,5 kWh');
    expect(infoRow('Çatı GES aylık üretimi')).toHaveTextContent('320 kWh (1 / 2 bina)');
  });

  it('says "veri yok" for a missing figure, never 0', () => {
    renderWithProviders(<MonthlyReportView payload={{ ...demoMonthly, utility: { ...demoMonthly.utility, value: undefined, with_data: 0 } }} />);
    expect(infoRow('Arazi GES aylık üretimi')).toHaveTextContent('veri yok');
  });

  it('prints one bill line per currency (R270)', () => {
    renderWithProviders(
      <MonthlyReportView
        payload={{ ...demoMonthly, bill: [...demoMonthly.bill, { currency: 'USD', value: '100', with_data: 1, of: 2 }] }}
      />,
    );
    const bill = infoRow('Elektrik faturası');
    expect(bill).toHaveTextContent('₺48.250,75');
    expect(bill).toHaveTextContent('100,00 USD');
  });

  it('shows four cards with their year-over-year change', () => {
    renderWithProviders(<MonthlyReportView payload={demoMonthly} />);
    for (const card of ['Toplam tüketim', 'Toplam üretim', 'Toplam fatura', 'Reaktif ceza']) {
      expect(screen.getAllByText(card).length).toBeGreaterThan(0);
    }
    expect(screen.getAllByText('geçen yılın aynı ayına göre').length).toBe(3); // the reactive delta is undefined
  });

  it('draws both charts with their titles', () => {
    renderWithProviders(<MonthlyReportView payload={demoMonthly} />);
    expect(screen.getAllByText('Aylık elektrik tüketimi')[0]).toBeVisible();
    expect(screen.getAllByText('Aylık elektrik faturası')[0]).toBeVisible();
  });

  it('labels an incomplete period (R274)', () => {
    renderWithProviders(<MonthlyReportView payload={{ ...demoMonthly, partial: true }} />);
    expect(screen.getByText(/henüz kapanmadı/)).toBeVisible();
  });
});
