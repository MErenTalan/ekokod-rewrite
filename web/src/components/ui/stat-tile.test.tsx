import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { StatTile } from './stat-tile';

describe('StatTile', () => {
  it('shows label, preformatted value and unit symbol', () => {
    const { getByText } = renderWithProviders(<StatTile label="Toplam tüketim" value="182.345,1" unit="kWh" hint="Geçen aya göre" />);
    expect(getByText('182.345,1')).toHaveClass('type-metric');
    expect(getByText('kWh')).toBeInTheDocument();
    expect(getByText('Geçen aya göre')).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<StatTile label="Toplam" value="12" unit="percent" />);
    await expectNoAxeViolations(container);
  });
});
