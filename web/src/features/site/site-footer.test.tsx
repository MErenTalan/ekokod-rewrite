import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { SiteFooter } from './site-footer';

describe('SiteFooter', () => {
  it('carries the contact details, the year and the flagged pricing link', () => {
    const { getByRole, getByText, queryByRole, unmount } = renderWithProviders(<SiteFooter year={2026} pricing={false} />);
    expect(getByRole('contentinfo')).toHaveAccessibleName('Site alt bilgisi');
    expect(getByRole('link', { name: 'info@ekokod.com' })).toHaveAttribute('href', 'mailto:info@ekokod.com');
    expect(getByRole('link', { name: '+90 506 315 41 98' })).toHaveAttribute('href', 'tel:+905063154198');
    expect(getByText('© 2026 EkoKod. Tüm hakları saklıdır.')).toBeInTheDocument();
    expect(queryByRole('link', { name: 'Fiyatlandırma' })).toBeNull();
    unmount();
    renderWithProviders(<SiteFooter year={2026} pricing />);
    expect(getByRole('link', { name: 'Fiyatlandırma' })).toHaveAttribute('href', '/pricing');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<SiteFooter year={2026} pricing />);
    await expectNoAxeViolations(container);
  });
});
