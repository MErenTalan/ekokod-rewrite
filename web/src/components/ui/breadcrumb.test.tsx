import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Breadcrumb } from './breadcrumb';

const items = [{ label: 'Veri Analizi', href: '/consumption' }, { label: 'Tüketim', href: '/consumption' }, { label: 'Merkez Bina' }];

describe('Breadcrumb', () => {
  it('last item is the current page', () => {
    const { getByRole, getByText } = renderWithProviders(<Breadcrumb items={items} />);
    expect(getByRole('navigation', { name: 'İçerik haritası' })).toBeInTheDocument();
    const last = getByText('Merkez Bina');
    expect(last).toHaveAttribute('aria-current', 'page');
    expect(last.closest('a')).toBeNull();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Breadcrumb items={items} />);
    await expectNoAxeViolations(container);
  });
});
