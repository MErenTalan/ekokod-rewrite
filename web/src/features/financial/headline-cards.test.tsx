import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { summary } from './_fixture';
import { HeadlineCards } from './headline-cards';

describe('HeadlineCards', () => {
  it('counts, the period figures per currency and the coverage "12 ayın 9\'u"', () => {
    const r = renderWithProviders(<HeadlineCards summary={summary} />);
    expect(r.getByText('14')).toBeVisible();
    expect(r.getByText('3.600')).toBeVisible();
    expect(r.getAllByText('120,00 EUR').length).toBeGreaterThan(0);
    expect(r.getByText("12 ayın 9'u veri içeriyor")).toBeVisible();
    expect(r.getByText('Bazı santrallerin satış tarifesi yok; gelir eksik.')).toBeVisible();
  });

  it('the English coverage line has no Turkish suffix', () => {
    const r = renderWithProviders(<HeadlineCards summary={summary} />, { locale: 'en' });
    expect(r.getByText('9 of 12 months have data')).toBeVisible();
  });
});
