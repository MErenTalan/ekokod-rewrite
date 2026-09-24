import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { TopNav } from './top-nav';

describe('TopNav', () => {
  it('groups are menus and disabled items say why', async () => {
    const { getByRole, findByRole, user } = renderWithProviders(<TopNav />);
    await user.click(getByRole('button', { name: 'Veri Analizi' }));
    const water = await findByRole('menuitem', { name: /Su/ });
    expect(water).toHaveAttribute('aria-disabled', 'true');
    expect(water).toHaveAccessibleDescription('Bu bölüm henüz kullanıma açılmadı');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<TopNav />);
    await expectNoAxeViolations(container);
  });
});
