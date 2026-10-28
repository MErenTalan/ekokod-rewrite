import { describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { AppShell } from './app-shell';

const user = { name: 'Ayşe Yılmaz', email: 'ayse@ornek.com.tr', roleLabel: 'Şirket Yöneticisi' };

describe('AppShell', () => {
  it('a demo session sees the reset banner above every page (R552)', () => {
    const me: MeResponse = {
      id: 'u-demo', name: 'Demo Kullanıcı', email: 'demo@ekokod.com.tr', role: 'demo', locale: 'tr',
      permissions: fixture.demo as MeResponse['permissions'], session_id: 's-1', company: { id: 'c-demo', name: 'Demo Enerji A.Ş.' },
      demo: { resets_at: '2026-10-28T12:00:00Z' },
    };
    const r = renderWithProviders(
      <SessionProvider me={me}>
        <AppShell>
          <p>İçerik</p>
        </AppShell>
      </SessionProvider>,
    );
    expect(r.getByRole('status')).toHaveTextContent('Sonraki sıfırlama: 15:00');
    expect(r.getByRole('main')).toHaveTextContent('Demo hesabı');
  });

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
