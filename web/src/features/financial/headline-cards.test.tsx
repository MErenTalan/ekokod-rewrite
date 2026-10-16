import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { monthly, summary } from './_fixture';
import { HeadlineCards } from './headline-cards';

describe('HeadlineCards', () => {
  it('counts, the period figures per currency and the coverage "12 ayın 9\'u"', () => {
    const r = renderWithProviders(<HeadlineCards summary={summary} />);
    expect(r.getByText('14')).toBeVisible();
    expect(r.getByText('3.600')).toBeVisible();
    expect(r.getAllByText('120,00 EUR').length).toBeGreaterThan(0);
    expect(r.getByText("12 ayın 9'u fatura içeriyor")).toBeVisible();
    expect(r.getByText('Faturasız aylarda tüketim ve üretim sayaç ölçümünden.')).toBeVisible();
    expect(r.getByText('Üretimin bir kısmının satış fiyatı yok; gelir eksik.')).toBeVisible();
  });

  it('a month without invoices says its figures are metered', () => {
    const r = renderWithProviders(
      <HeadlineCards summary={{ ...summary, of: 1, with_data: 0, figures: monthly.items[3]! }} />,
    );
    expect(r.getByText('420')).toBeVisible();
    expect(r.getByText('Bu ay için fatura yok; tüketim ve üretim sayaç ölçümünden.')).toBeVisible();
  });

  it('the English coverage line has no Turkish suffix', () => {
    const r = renderWithProviders(<HeadlineCards summary={summary} />, { locale: 'en' });
    expect(r.getByText('9 of 12 months have invoices')).toBeVisible();
  });
});
