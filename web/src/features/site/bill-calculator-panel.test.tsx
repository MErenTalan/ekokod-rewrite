import { afterEach, describe, expect, it } from 'vitest';

import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { BillCalculatorPanel } from './bill-calculator-panel';

let api: ReturnType<typeof mockApi>;
afterEach(() => api?.restore());

const bill = {
  basis: 'total', days: 31, energy: '100.00', distribution: '50.00', power: '0.00', overuse: '0.00', vat_base: '150.00', vat: '30.00', total: '180.00',
  vat_rate: '20.000', tariff: { effective_from: '2025-01-01', group_used: 'residential' },
};

describe('BillCalculatorPanel', () => {
  it('prices the entered period and shows the breakdown', async () => {
    let sent: Record<string, unknown> = {};
    api = mockApi({ 'POST /api/v1/public/bill-calculator': (req: Request) => req.json().then((b: Record<string, unknown>) => ((sent = b), Response.json(bill))) });
    const r = renderWithProviders(<BillCalculatorPanel today="2026-09-24" />);
    await r.user.type(r.getByLabelText(/Toplam tüketim/), '250');
    await r.user.click(r.getByRole('button', { name: 'Hesapla' }));
    expect(await r.findByText('₺180,00')).toBeInTheDocument();
    expect(sent).toEqual({ user_group: 'residential', voltage_level: 'lv', term: 'monomial', multi_time: false, start: '2026-08-01', end: '2026-08-31', total_consumption: '250' });
  });

  it.each([
    ['total_consumption', 'mismatch', /Toplam tüketim/, "Toplam tüketim T1, T2 ve T3'ün toplamına eşit olmalı."],
    ['start', 'no_tariff', /Başlangıç tarihi/, 'Bu tarih için yayımlanmış bir tarife yok.'],
  ])('puts a %s %s refusal on its field', async (field, code, label, message) => {
    api = mockApi({ 'POST /api/v1/public/bill-calculator': Response.json({ error: { code: 'validation_failed', message: 'x', details: { [field]: [code] } } }, { status: 422 }) });
    const r = renderWithProviders(<BillCalculatorPanel today="2026-09-24" />);
    await r.user.type(r.getByLabelText(/Toplam tüketim/), '250');
    await r.user.click(r.getByRole('button', { name: 'Hesapla' }));
    expect(await r.findByText(message)).toBeInTheDocument();
    expect(r.getAllByLabelText(label)[0]).toHaveAccessibleDescription(expect.stringContaining(message));
  });
});
