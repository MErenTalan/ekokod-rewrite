import { afterEach, describe, expect, it, vi } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

const getSession = vi.hoisted(() => vi.fn<() => Promise<{ me: MeResponse } | { me: null; code: string }>>());
vi.mock('@/lib/api/server', () => ({ getSession }));
const modulesFor = vi.hoisted(() => vi.fn());
vi.mock('@/modules/access', () => ({ modulesFor }));
const redirect = vi.hoisted(() =>
  vi.fn((href: string) => {
    throw new Error(`NEXT_REDIRECT ${href}`);
  }),
);
vi.mock('next/navigation', async () => {
  const { mockRouter } = await import('@/test/navigation');
  return { redirect, useRouter: () => mockRouter, usePathname: () => '/modules' };
});

const { default: ModulesPage } = await import('./page');

const me: MeResponse = {
  id: 'u1',
  name: 'Ayşe Yılmaz',
  email: 'ayse@ornek.com.tr',
  role: 'company_admin',
  locale: 'tr',
  permissions: fixture.company_admin as MeResponse['permissions'],
  session_id: 's1',
  company: { id: 'c1', name: 'Anadolu Tekstil' },
};

afterEach(() => {
  getSession.mockReset();
  modulesFor.mockReset();
});

describe('/modules (F17 R495)', () => {
  it('sends a visitor without a session to login with the chooser as next', async () => {
    getSession.mockResolvedValue({ me: null, code: 'session_revoked' });
    await expect(ModulesPage()).rejects.toThrow('NEXT_REDIRECT /auth/login?next=%2Fmodules');
  });

  it('offers every module the user may open, each linking to its home', async () => {
    getSession.mockResolvedValue({ me });
    modulesFor.mockReturnValue(['ekorm', 'ekocm']);
    const r = renderWithProviders(await ModulesPage());
    expect(r.getByRole('heading', { level: 1, name: 'Modül seçin' })).toBeVisible();
    expect(r.getByText(/Hoş geldiniz, Ayşe Yılmaz/)).toBeVisible();
    expect(r.getByRole('link', { name: 'EKO-RM modülünü aç' })).toHaveAttribute('href', '/ekorm');
    expect(r.getByRole('link', { name: 'EKO-CM modülünü aç' })).toHaveAttribute('href', '/ekocm');
    await expectNoAxeViolations(r.container);
  });

  it('skips the chooser when the user has a single module (U3)', async () => {
    getSession.mockResolvedValue({ me });
    modulesFor.mockReturnValue(['ekocm']);
    await expect(ModulesPage()).rejects.toThrow('NEXT_REDIRECT /ekocm');
  });
});
