import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { mockPathname, mockRouter } from '@/test/navigation';
import { renderWithProviders } from '@/test/render';

const searchParams = vi.hoisted(() => ({ current: new URLSearchParams() }));
vi.mock('next/navigation', async () => {
  const { mockPathname: pathname, mockRouter: router } = await import('@/test/navigation');
  return {
    usePathname: () => pathname.current,
    useRouter: () => router,
    useSearchParams: () => searchParams.current,
    redirect: vi.fn(),
  };
});

const { SettingsPage } = await import('./settings-page');

const me = (role: keyof typeof fixture): MeResponse => ({
  id: `u-${role}`,
  name: 'Ayşe Kaya',
  email: 'ayse@ornek.com.tr',
  role,
  locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'],
  session_id: 's-1',
  company: { id: 'c-1', name: 'Anadolu Tekstil' },
});

const ROUTES = {
  'GET /api/v1/auth/sessions': { items: [] },
  'GET /api/v1/smtp-settings': () => Response.json({ error: { code: 'not_found', message: 'yok' } }, { status: 404 }),
  'GET /api/v1/integration-definitions': { items: [] },
};

let api: ReturnType<typeof mockApi>;
beforeEach(() => {
  mockPathname.current = '/ekorm/settings';
  searchParams.current = new URLSearchParams();
  mockRouter.replace.mockClear();
});
afterEach(() => api.restore());

describe('SettingsPage', () => {
  it('shows a company admin only the tabs their role has', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <SettingsPage />
      </SessionProvider>,
    );
    const tabs = r.getAllByRole('tab').map((tab) => tab.textContent);
    expect(tabs).toEqual(['Hesap', 'Şirket', 'Binalar', 'Güneş Santralleri', 'Analizörler', 'Kullanıcılar']);
  });

  it('corrects a tab the role may not open', async () => {
    searchParams.current = new URLSearchParams('tab=smtp');
    api = mockApi(ROUTES);
    renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <SettingsPage />
      </SessionProvider>,
    );
    await waitFor(() => expect(mockRouter.replace).toHaveBeenCalledWith('/ekorm/settings?tab=account'));
  });

  it('opens the tab the URL asks for when the role has it', async () => {
    searchParams.current = new URLSearchParams('tab=smtp');
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('admin')}>
        <SettingsPage />
      </SessionProvider>,
    );
    await waitFor(() => expect(r.getByRole('tab', { name: 'SMTP Ayarları' })).toHaveAttribute('aria-selected', 'true'));
    expect(mockRouter.replace).not.toHaveBeenCalled();
  });

  it('gives the demo account one read-only tab and asks for no sessions', async () => {
    api = mockApi(ROUTES);
    const r = renderWithProviders(
      <SessionProvider me={me('demo')}>
        <SettingsPage />
      </SessionProvider>,
    );
    expect(r.getAllByRole('tab')).toHaveLength(1);
    expect(r.getByText('Demo hesabı paylaşımlıdır; profil ve şifre değiştirilemez.')).toBeInTheDocument();
    await waitFor(() => expect(api.calls.every((c) => !c.url.includes('/auth/sessions'))).toBe(true));
  });
});
