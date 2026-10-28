import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { DemoBanner } from './demo-banner';

// 12:00 UTC is 15:00 in Istanbul (R552).
const RESETS_AT = '2026-10-28T12:00:00Z';

describe('DemoBanner', () => {
  it('says the data is sample data, resets hourly, and when the next reset lands', () => {
    const { getByRole } = renderWithProviders(<DemoBanner resetsAt={RESETS_AT} />);
    const banner = getByRole('status');
    expect(banner).toHaveTextContent('Demo hesabı');
    expect(banner).toHaveTextContent(
      'Bu hesaptaki veriler örnektir. Yaptığınız değişiklikler kalıcı değildir; hesap her saat başı ilk haline döner.',
    );
    expect(banner).toHaveTextContent('Sonraki sıfırlama: 15:00');
  });

  it('renders nothing without a reset time', () => {
    const { queryByRole, queryByText } = renderWithProviders(<DemoBanner />);
    expect(queryByRole('status')).toBeNull();
    expect(queryByText('Demo hesabı')).toBeNull();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<DemoBanner resetsAt={RESETS_AT} />);
    await expectNoAxeViolations(container);
  });
});
