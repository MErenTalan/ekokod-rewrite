import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { AppShell } from './app-shell';

const user = { name: 'Ayşe Yılmaz', email: 'ayse@ornek.com.tr', roleLabel: 'Şirket Yöneticisi' };

describe('AppShell', () => {
  it('skip link moves focus to main', async () => {
    const r = renderWithProviders(
      <AppShell user={user}>
        <p>İçerik</p>
      </AppShell>,
    );
    const skip = r.getByRole('link', { name: 'İçeriğe geç' });
    skip.focus();
    await r.user.keyboard('{Enter}');
    expect(document.getElementById('main-content')).toHaveFocus();
  });

  it('the sidebar toggle opens a dialog', async () => {
    const r = renderWithProviders(
      <AppShell user={user} notificationCount={3}>
        <p>İçerik</p>
      </AppShell>,
    );
    await r.user.click(r.getByRole('button', { name: 'Menüyü Aç/Kapat' }));
    expect(await r.findByRole('dialog', { name: 'Ana menü' })).toBeInTheDocument();
  });

  it('horizontal layout renders the top navigation instead of the docked sidebar', () => {
    const r = renderWithProviders(
      <AppShell user={user}>
        <p>İçerik</p>
      </AppShell>,
      { preferences: { layout: 'horizontal' } },
    );
    expect(r.container.querySelector('[data-top-nav]')).not.toBeNull();
    expect(r.container.querySelector('aside[data-docked-sidebar]')).toBeNull();
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(
      <AppShell user={user}>
        <h1>Sayfa</h1>
      </AppShell>,
    );
    await expectNoAxeViolations(r.container);
  });
});
