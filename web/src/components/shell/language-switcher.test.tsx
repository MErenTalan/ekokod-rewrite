import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { mockRouter } from '@/test/navigation';
import { renderWithProviders } from '@/test/render';

import { LanguageSwitcher } from './language-switcher';

const setLocale = vi.hoisted(() => vi.fn(async () => {}));
vi.mock('@/i18n/actions', () => ({ setLocale }));

describe('LanguageSwitcher', () => {
  it('calls setLocale then refresh', async () => {
    const { getByRole, findByRole, user } = renderWithProviders(<LanguageSwitcher />);
    await user.click(getByRole('button', { name: 'Dil' }));
    await user.click(await findByRole('menuitem', { name: /English/ }));
    await vi.waitFor(() => expect(mockRouter.refresh).toHaveBeenCalled());
    expect(setLocale).toHaveBeenCalledWith('en');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<LanguageSwitcher />);
    await expectNoAxeViolations(container);
  });
});
