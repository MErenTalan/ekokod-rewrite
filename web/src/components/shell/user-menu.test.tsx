import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { UserMenu } from './user-menu';

const user = { name: 'Ayşe Yılmaz', email: 'ayse@ornek.com.tr', roleLabel: 'Şirket yöneticisi' };

describe('UserMenu', () => {
  it('shows identity and opens display settings', async () => {
    const onOpenCustomizer = vi.fn();
    const r = renderWithProviders(<UserMenu user={user} onOpenCustomizer={onOpenCustomizer} />);
    await r.user.click(r.getByRole('button', { name: 'Kullanıcı menüsü: Ayşe Yılmaz' }));
    const menu = await r.findByRole('menu');
    expect(menu).toHaveTextContent('Şirket yöneticisi');
    await r.user.click(r.getByRole('menuitem', { name: 'Görünüm ayarları' }));
    expect(onOpenCustomizer).toHaveBeenCalled();
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(<UserMenu user={user} />);
    await expectNoAxeViolations(r.container);
  });
});
