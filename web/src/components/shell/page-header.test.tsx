import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { setMockPathname } from '@/test/navigation';
import { renderWithProviders } from '@/test/render';

import { PageHeader } from './page-header';

describe('PageHeader', () => {
  it('title is the page h1 and the breadcrumb is derived from the route', () => {
    setMockPathname('/ekorm/load-profile');
    const { getByRole, getByText } = renderWithProviders(<PageHeader title="Yük Profili" description="Merkez Bina" />);
    expect(getByRole('heading', { level: 1, name: 'Yük Profili' })).toBeInTheDocument();
    expect(getByRole('navigation', { name: 'İçerik haritası' })).toHaveTextContent('Veri Analizi');
    expect(getByText('Merkez Bina')).toBeInTheDocument();
    setMockPathname('/');
  });

  it('an explicit breadcrumb overrides the derived one', () => {
    const { getByRole } = renderWithProviders(<PageHeader title="Fatura" breadcrumb={[{ label: 'Faturalar', href: '/bills' }, { label: 'Ağustos 2026' }]} />);
    expect(getByRole('navigation', { name: 'İçerik haritası' })).toHaveTextContent('Ağustos 2026');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<PageHeader title="Başlık" actions={<button type="button">Ekle</button>} />);
    await expectNoAxeViolations(container);
  });
});
