import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { ThemeToggle } from './theme-toggle';

describe('ThemeToggle', () => {
  it('cycles light, dark, system and names the current mode', async () => {
    const { getByRole, user } = renderWithProviders(<ThemeToggle />, { preferences: { theme: 'light' } });
    await user.click(getByRole('button', { name: 'Tema: Aydınlık' }));
    expect(getByRole('button', { name: 'Tema: Karanlık' })).toBeInTheDocument();
    await user.click(getByRole('button', { name: 'Tema: Karanlık' }));
    expect(getByRole('button', { name: 'Tema: Sistem' })).toBeInTheDocument();
    expect(document.documentElement.dataset.theme).toBe('system');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<ThemeToggle />);
    await expectNoAxeViolations(container);
  });
});
