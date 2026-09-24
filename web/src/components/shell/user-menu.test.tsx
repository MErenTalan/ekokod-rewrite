import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { mockRouter } from '@/test/navigation';
import { renderWithProviders } from '@/test/render';

import { UserMenu } from './user-menu';

const user = { name: 'Ayşe Yılmaz', email: 'ayse@ornek.com.tr', roleLabel: 'Şirket Yöneticisi' };

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  mockRouter.replace.mockClear();
});
afterEach(() => vi.unstubAllGlobals());

describe('UserMenu', () => {
  it('shows identity and opens display settings', async () => {
    const onOpenCustomizer = vi.fn();
    const r = renderWithProviders(<UserMenu user={user} onOpenCustomizer={onOpenCustomizer} />);
    await r.user.click(r.getByRole('button', { name: 'Kullanıcı menüsü: Ayşe Yılmaz' }));
    const menu = await r.findByRole('menu');
    expect(menu).toHaveTextContent('Şirket Yöneticisi');
    await r.user.click(r.getByRole('menuitem', { name: 'Görünüm ayarları' }));
    expect(onOpenCustomizer).toHaveBeenCalled();
  });

  it('logout ends the session and returns to login', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    const r = renderWithProviders(<UserMenu user={user} />);
    await r.user.click(r.getByRole('button', { name: 'Kullanıcı menüsü: Ayşe Yılmaz' }));
    await r.user.click(await r.findByRole('menuitem', { name: 'Çıkış' }));
    await waitFor(() => expect(mockRouter.replace).toHaveBeenCalledWith('/auth/login'));
    const request = fetchMock.mock.calls[0][0] as Request;
    expect(request.method).toBe('POST');
    expect(new URL(request.url).pathname).toBe('/api/v1/auth/logout');
  });

  it('stays put and says so when logout fails', async () => {
    fetchMock.mockRejectedValue(new TypeError('offline'));
    const r = renderWithProviders(<UserMenu user={user} />);
    await r.user.click(r.getByRole('button', { name: 'Kullanıcı menüsü: Ayşe Yılmaz' }));
    await r.user.click(await r.findByRole('menuitem', { name: 'Çıkış' }));
    expect(await r.findByText('Çıkış yapılamadı. Lütfen tekrar deneyin.')).toBeInTheDocument();
    expect(mockRouter.replace).not.toHaveBeenCalled();
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(<UserMenu user={user} />);
    await expectNoAxeViolations(r.container);
  });
});
