import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { TopBar } from './top-bar';

const props = {
  user: { name: 'Ayşe Yılmaz', email: 'ayse@ornek.com.tr', roleLabel: 'Yönetici' },
  sidebarOpen: false,
  onToggleSidebar: vi.fn(),
  onOpenCustomizer: vi.fn(),
};

describe('TopBar', () => {
  it('search has a hidden label and notifications announce the count', () => {
    const { getAllByRole, getByRole } = renderWithProviders(<TopBar {...props} notificationCount={4} />);
    expect(getAllByRole('searchbox', { name: 'Ara' }).length).toBeGreaterThan(0);
    expect(getByRole('button', { name: 'Bildirimler (4 yeni)' })).toBeInTheDocument();
    expect(getByRole('button', { name: 'Menüyü Aç/Kapat' })).toHaveAttribute('aria-expanded', 'false');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<TopBar {...props} />);
    await expectNoAxeViolations(container);
  });
});
