import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { setMatchMedia } from '@/test/match-media';
import { renderWithProviders } from '@/test/render';

import { actuals, forecast } from './_fixture';
import { ForecastView } from './forecast-view';

describe('ForecastView', () => {
  it('draws actuals, the dashed neutral median and the p10–p90 band (R381)', () => {
    // The draw animation owns stroke-dasharray until it ends, and jsdom never advances it.
    setMatchMedia((q) => q.includes('reduce'));
    const { getByRole, container } = renderWithProviders(<ForecastView actuals={actuals} forecast={forecast} unavailable={false} loading={false} />);
    setMatchMedia(() => false);
    expect(getByRole('figure', { name: 'Gerçekleşen ve tahmin' })).toBeInTheDocument();
    const curves = container.querySelectorAll('.recharts-line-curve');
    expect(curves).toHaveLength(2);
    expect(curves[1]).toHaveAttribute('stroke', 'var(--color-forecast)');
    expect(curves[1]).toHaveAttribute('stroke-dasharray', '6 4');
    expect(container.querySelector('.recharts-area-area')).not.toBeNull();
  });

  it('names the model, the covariates and a fallback (R383)', () => {
    const { getByText, rerender } = renderWithProviders(<ForecastView actuals={actuals} forecast={forecast} unavailable={false} loading={false} />);
    expect(getByText(/Model: random_forest 1\.0\.0/)).toBeInTheDocument();
    expect(getByText('Kullanılan değişkenler: gün tipi, tatil')).toBeInTheDocument();
    rerender(<ForecastView actuals={actuals} forecast={{ ...forecast, model_id: 'seasonal_naive', fallback_from: 'random_forest' }} unavailable={false} loading={false} />);
    expect(getByText('random_forest kullanılamadı; temel istatistiksel model kullanıldı.')).toBeInTheDocument();
  });

  it('surfaces the reported gaps with their hours (R382)', () => {
    const { getByText, getByRole } = renderWithProviders(<ForecastView actuals={actuals} forecast={forecast} unavailable={false} loading={false} />);
    expect(getByText(/Geçmiş veride 1 boşluk var \(3 saat\)/)).toBeInTheDocument();
    const table = getByRole('table', { name: 'Verideki boşluklar' });
    expect(table).toHaveTextContent('3');
  });

  it.each([
    [{ ...forecast, status: 'insufficient_data' as const, points: [] }, false, /Geçmiş veri yetersiz/],
    [{ ...forecast, status: 'no_data' as const, points: [], gaps: [] }, false, /geçmiş tüketim verisi yok/],
    [{ ...forecast, status: 'model_error' as const, points: [], gaps: [] }, false, /Model tahmini üretemedi/],
    [{ ...forecast, points: [], gaps: [] }, false, /Model sonuç döndürmedi/],
    [null, false, /kayıtlı tahmin yok/],
    [forecast, true, /Tahmin servisi şu anda kullanılamıyor/],
  ])('explains every status (R380) %#', (f, unavailable, message) => {
    const { getByText } = renderWithProviders(<ForecastView actuals={actuals} forecast={f} unavailable={unavailable} loading={false} />);
    expect(getByText(message)).toBeInTheDocument();
  });

  it('keeps showing actuals when the service is down (R385)', () => {
    const { getByRole } = renderWithProviders(<ForecastView actuals={actuals} forecast={null} unavailable loading={false} />);
    expect(getByRole('figure', { name: 'Gerçekleşen ve tahmin' })).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<ForecastView actuals={actuals} forecast={forecast} unavailable={false} loading={false} />);
    await expectNoAxeViolations(container);
  });
});
