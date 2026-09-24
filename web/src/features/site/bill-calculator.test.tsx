import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { BillCalculator, type BillCalculatorProps } from './bill-calculator';
import { initialValues } from './calculator-form';

const result = {
  basis: 'total' as const, days: 31, energy: '30000.00', distribution: '12000.00', power: '7500.00', overuse: '12743.00', vat_base: '62243.00',
  vat: '12448.60', total: '74691.60', vat_rate: '20.000', tariff: { effective_from: '2025-01-01', group_used: 'commercial_plus' },
};
const props = (over: Partial<BillCalculatorProps> = {}): BillCalculatorProps => ({
  values: initialValues('2026-09-24'), errors: {}, result: null, pending: false, onChange: vi.fn(), onSubmit: vi.fn(), ...over,
});

describe('BillCalculator', () => {
  it('offers double term only at medium voltage and asks for power there', () => {
    const lv = renderWithProviders(<BillCalculator {...props()} />);
    expect(lv.getByRole('radio', { name: 'Çift terimli' })).toBeDisabled();
    expect(lv.queryByLabelText(/Sözleşme gücü/)).toBeNull();
    lv.unmount();
    const mv = renderWithProviders(<BillCalculator {...props({ values: { ...initialValues('2026-09-24'), voltage: 'mv', term: 'binomial' } })} />);
    expect(mv.getByRole('radio', { name: 'Çift terimli' })).toBeEnabled();
    expect(mv.getByLabelText(/Sözleşme gücü/)).toBeInTheDocument();
    expect(mv.getByLabelText(/Ölçülen güç/)).toBeInTheDocument();
  });

  it('has no multi-time tariff for lighting', () => {
    const r = renderWithProviders(<BillCalculator {...props({ values: { ...initialValues('2026-09-24'), group: 'lighting' } })} />);
    expect(r.getByRole('radio', { name: 'Çok zamanlı (T1/T2/T3)' })).toBeDisabled();
  });

  it('breaks the bill down line by line, power charge included', () => {
    const r = renderWithProviders(<BillCalculator {...props({ result })} />);
    const out = r.getByRole('region', { name: 'Fatura detayı' });
    expect(out).toHaveTextContent(/Güç bedeli\s*₺7\.500,00/);
    expect(out).toHaveTextContent(/Güç aşım bedeli\s*₺12\.743,00/);
    expect(out).toHaveTextContent(/KDV \(%20\)\s*₺12\.448,60/);
    expect(out).toHaveTextContent(/Toplam fatura\s*₺74\.691,60/);
    expect(out).toHaveTextContent('31 günlük dönem');
    expect(out).toHaveTextContent('yüksek tüketim fiyatı');
    expect(out).toHaveTextContent('Enerji toplam tüketim üzerinden fiyatlandırıldı.');
  });

  it('documents the total vs T1/T2/T3 and power-charge rules (Q-H3, Q-H4)', () => {
    const r = renderWithProviders(<BillCalculator {...props()} />);
    expect(r.getByText(/toplam da girilirse üçünün toplamına eşit olmalıdır/)).toBeInTheDocument();
    expect(r.getByText(/sözleşme gücü × gün sayısı \/ 30/)).toBeInTheDocument();
  });

  it('reports changes and submits', async () => {
    const onChange = vi.fn();
    const onSubmit = vi.fn();
    const r = renderWithProviders(<BillCalculator {...props({ onChange, onSubmit })} />);
    await r.user.click(r.getByRole('radio', { name: 'Orta gerilim (OG)' }));
    expect(onChange).toHaveBeenCalledWith({ voltage: 'mv' });
    await r.user.click(r.getByRole('button', { name: 'Hesapla' }));
    expect(onSubmit).toHaveBeenCalled();
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(<BillCalculator {...props({ result, errors: { total: 'Zorunlu alan' } })} />);
    await expectNoAxeViolations(r.container);
  });
});
