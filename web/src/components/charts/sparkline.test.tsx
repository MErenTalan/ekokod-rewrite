import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Sparkline } from './sparkline';

describe('Sparkline', () => {
  it('summary label', () => {
    const { getByRole } = renderWithProviders(<Sparkline label="Günlük tüketim" data={['120.5', '98', null, '143.25', '110']} kind="consumption" />);
    const img = getByRole('img', { name: /en düşük/i });
    expect(img).toHaveAccessibleName('Günlük tüketim: İlk 120,5, son 110, en düşük 98, en yüksek 143,25');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Sparkline label="Tüketim" data={['1', '2']} kind="consumption" />);
    await expectNoAxeViolations(container);
  });
});
