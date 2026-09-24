import { afterEach, describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { setMockPathname } from '@/test/navigation';
import { renderWithProviders } from '@/test/render';

import { SiteHeader } from './site-header';

afterEach(() => setMockPathname('/'));

describe('SiteHeader', () => {
  it('links every public page and hides pricing behind its flag', () => {
    const { getByRole, queryByRole, unmount } = renderWithProviders(<SiteHeader pricing={false} signedIn={false} />);
    const nav = getByRole('navigation', { name: 'Site menüsü' });
    expect(nav).toBeInTheDocument();
    expect(getByRole('link', { name: 'Kurumsal' })).toHaveAttribute('href', '/about');
    expect(getByRole('link', { name: 'Fatura Hesaplama' })).toHaveAttribute('href', '/bill-calculator');
    expect(queryByRole('link', { name: 'Fiyatlandırma' })).toBeNull();
    unmount();
    renderWithProviders(<SiteHeader pricing signedIn={false} />);
    expect(getByRole('link', { name: 'Fiyatlandırma' })).toHaveAttribute('href', '/pricing');
  });

  it('marks the current section, including an article under the blog', () => {
    setMockPathname('/blog/elektrik-faturam-neden-yuksek-1');
    const { getByRole } = renderWithProviders(<SiteHeader pricing={false} signedIn={false} />);
    expect(getByRole('link', { name: 'Blog' })).toHaveAttribute('aria-current', 'page');
    expect(getByRole('link', { name: 'Kurumsal' })).not.toHaveAttribute('aria-current');
  });

  it('offers sign-in to visitors and the platform to signed-in users (Q-H11)', () => {
    const { getByRole, queryByRole, unmount } = renderWithProviders(<SiteHeader pricing={false} signedIn={false} />);
    expect(getByRole('link', { name: 'Giriş Yap' })).toHaveAttribute('href', '/auth/login');
    unmount();
    renderWithProviders(<SiteHeader pricing={false} signedIn />);
    expect(getByRole('link', { name: 'Platforma Git' })).toHaveAttribute('href', '/ekorm');
    expect(queryByRole('link', { name: 'Giriş Yap' })).toBeNull();
  });

  it('opens a drawer with the same links on small screens', async () => {
    const { getByRole, findByRole, user } = renderWithProviders(<SiteHeader pricing signedIn={false} />);
    await user.click(getByRole('button', { name: 'Menüyü aç' }));
    const drawer = await findByRole('dialog', { name: 'Menü' });
    expect(drawer).toHaveTextContent('Fiyatlandırma');
    expect(drawer).toHaveTextContent('Demo Talep Et');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<SiteHeader pricing signedIn={false} />);
    await expectNoAxeViolations(container);
  });
});
