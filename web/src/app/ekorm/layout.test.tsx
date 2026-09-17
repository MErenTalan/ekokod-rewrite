import { afterEach, describe, expect, it, vi } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { mockPathname, mockRouter } from '@/test/navigation';
import { renderWithProviders } from '@/test/render';

const getSession = vi.hoisted(() => vi.fn<() => Promise<{ me: MeResponse } | { me: null; code: string }>>());
vi.mock('@/lib/api/server', () => ({ getSession }));
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

const { default: EkormLayout } = await import('./layout');

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
  vi.unstubAllGlobals();
});

describe('/ekorm layout', () => {
  it('sends a user without a session to login', async () => {
    getSession.mockResolvedValue({ me: null, code: 'session_revoked' });
    await expect(EkormLayout({ children: null })).rejects.toThrow('NEXT_REDIRECT /auth/login');
  });

  it('explains a device mismatch on the way to login', async () => {
    getSession.mockResolvedValue({ me: null, code: 'device_mismatch' });
    await expect(EkormLayout({ children: null })).rejects.toThrow('NEXT_REDIRECT /auth/login?reason=device_mismatch');
  });

  it('renders the signed-in user with the role label', async () => {
    getSession.mockResolvedValue({ me: me('company_admin') });
    const r = renderWithProviders(await EkormLayout({ children: <p>İçerik</p> }));
    await r.user.click(r.getByRole('button', { name: 'Kullanıcı menüsü: Ayşe Yılmaz' }));
    expect(await r.findByRole('menu')).toHaveTextContent('Şirket Yöneticisi · ayse@ornek.com.tr');
    expect(r.queryByRole('combobox', { name: 'Şirket' })).toBeNull();
  });

  it('filters the navigation by permission', async () => {
    getSession.mockResolvedValue({ me: me('building_admin') });
    const r = renderWithProviders(await EkormLayout({ children: null }));
    await r.user.click(r.getAllByRole('button', { name: 'Veri Analizi' })[0]);
    expect(r.getAllByRole('link', { name: 'Tüketim' }).length).toBeGreaterThan(0);
    expect(r.queryByRole('link', { name: 'GES Santralleri' })).toBeNull();
    expect(r.queryByRole('link', { name: 'Finansal Analiz' })).toBeNull();
  });

  it('gives an admin the company switcher', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => Response.json({ items: [] })));
    getSession.mockResolvedValue({ me: me('admin') });
    const r = renderWithProviders(await EkormLayout({ children: null }));
    expect(r.getByRole('combobox', { name: 'Şirket' })).toBeInTheDocument();
  });
});
