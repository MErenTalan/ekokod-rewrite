import { afterEach, describe, expect, it, vi } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { mockPathname, mockRouter } from '@/test/navigation';
import { renderWithProviders } from '@/test/render';

const getSession = vi.hoisted(() => vi.fn<() => Promise<{ me: MeResponse } | { me: null; code: string }>>());
vi.mock('@/lib/api/server', () => ({ getSession }));
const requestHeaders = vi.hoisted(() => ({ current: new Headers() }));
vi.mock('next/headers', () => ({ headers: async () => requestHeaders.current }));
const redirect = vi.hoisted(() =>
  vi.fn((href: string) => {
    throw new Error(`NEXT_REDIRECT ${href}`);
  }),
);
vi.mock('next/navigation', () => ({
  redirect,
  usePathname: () => mockPathname.current,
  useRouter: () => mockRouter,
  useSearchParams: () => new URLSearchParams(),
}));

const { default: EkocmLayout } = await import('./layout');

const me = (role: keyof typeof fixture): MeResponse => ({
  id: 'u1',
  name: 'Ayşe Yılmaz',
  email: 'ayse@ornek.com.tr',
  role,
  locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'],
  session_id: 's1',
  company: { id: 'c1', name: 'Anadolu Tekstil' },
});

afterEach(() => {
  getSession.mockReset();
  requestHeaders.current = new Headers();
  vi.unstubAllGlobals();
});

describe('/ekocm layout', () => {
  it('sends a user without a session to login, keeping the EKOCM page as next', async () => {
    requestHeaders.current = new Headers({ 'x-ekokod-path': '/ekocm/data-entry' });
    getSession.mockResolvedValue({ me: null, code: 'session_revoked' });
    await expect(EkocmLayout({ children: null })).rejects.toThrow('NEXT_REDIRECT /auth/login?next=%2Fekocm%2Fdata-entry');
  });

  it('renders the EKOCM shell: its brand, home and sidebar, none of EKORM’s (R489)', async () => {
    mockPathname.current = '/ekocm';
    getSession.mockResolvedValue({ me: me('company_admin') });
    const r = renderWithProviders(await EkocmLayout({ children: <p>İçerik</p> }));
    expect(r.getByRole('link', { name: 'EKO-CM ana sayfası' })).toHaveAttribute('href', '/ekocm');
    expect(r.container.querySelector('[data-module]')).toHaveAttribute('data-module', 'ekocm');
    expect(r.getAllByRole('link', { name: 'Genel Bakış' })[0]).toHaveAttribute('href', '/ekocm');
    expect(r.getAllByRole('link', { name: 'Raporlama' })[0]).toHaveAttribute('href', '/ekocm/reports');
    expect(r.queryByRole('link', { name: 'Yönetim Arayüzü' })).toBeNull();
    expect(r.queryByRole('button', { name: 'Veri Analizi' })).toBeNull();
    expect(r.getByText('İçerik')).toBeInTheDocument();
    expect(r.getByRole('button', { name: 'Modül değiştir' })).toBeInTheDocument();
  });

  it('shows no carbon navigation to a principal without carbon.read', async () => {
    mockPathname.current = '/ekocm';
    getSession.mockResolvedValue({ me: { ...me('company_admin'), permissions: ['nav.core'] } });
    const r = renderWithProviders(await EkocmLayout({ children: null }));
    expect(r.queryByRole('link', { name: 'Genel Bakış' })).toBeNull();
  });
});
