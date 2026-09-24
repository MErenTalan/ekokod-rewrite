import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { AnomalyView } from './anomaly-view';

const verdict = {
  available: true, is_anomaly: true, score: 7.52, actual: '40', expected: '10', lower: '4.81', upper: '15.19',
  method: 'robust_zscore_same_hour_of_week', model_id: 'robust_zscore_same_hour_of_week', model_version: '1.0.0',
};

describe('AnomalyView', () => {
  it('states the verdict, the score, the band and the method', () => {
    const { getByText, getByRole } = renderWithProviders(<AnomalyView result={verdict} />);
    expect(getByText('Anomali')).toBeInTheDocument();
    const table = getByRole('table', { name: 'Gerçekleşen ve beklenen' });
    expect(table).toHaveTextContent('7,52');
    expect(table).toHaveTextContent('4,81 – 15,19');
    expect(table).toHaveTextContent('Aynı haftanın aynı saatine göre sağlam z-skoru');
    expect(getByRole('figure', { name: 'Gerçekleşen ve beklenen' })).toBeInTheDocument();
  });

  it('says normal, and says when history was too short', () => {
    const { getByText, rerender } = renderWithProviders(<AnomalyView result={{ ...verdict, is_anomaly: false }} />);
    expect(getByText('Normal')).toBeInTheDocument();
    rerender(<AnomalyView result={{ available: true, is_anomaly: false, actual: '40', method: 'insufficient_history' }} />);
    expect(getByText('Karşılaştırma için yeterli geçmiş yok')).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<AnomalyView result={verdict} />);
    await expectNoAxeViolations(container);
  });
});
