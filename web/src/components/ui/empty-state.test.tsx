import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { EmptyState } from './empty-state';

describe('EmptyState', () => {
  it('description is required text', () => {
    const { getByText } = renderWithProviders(<EmptyState title="Fatura yok" description="Bir tarife atayın ve dönemi hesaplayın." />);
    expect(getByText('Fatura yok')).toBeVisible();
    expect(getByText('Bir tarife atayın ve dönemi hesaplayın.')).toBeVisible();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<EmptyState title="Fatura yok" description="Tarife atayın." action={<button type="button">Tarife ata</button>} />);
    await expectNoAxeViolations(container);
  });
});
