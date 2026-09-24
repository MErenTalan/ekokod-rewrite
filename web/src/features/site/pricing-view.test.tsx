import { within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { PricingView } from './pricing-view';

describe('PricingView', () => {
  it('compares three packages and marks the most popular one', () => {
    const { getAllByRole, getByRole } = renderWithProviders(<PricingView />);
    const cards = getAllByRole('article');
    expect(cards.map((c) => within(c).getByRole('heading', { level: 2 }).textContent)).toEqual(['Standart', 'Premium', 'Kurumsal']);
    const premium = getByRole('article', { name: /Premium/ });
    expect(premium).toHaveAccessibleDescription(/En çok tercih edilen/);
    expect(within(premium).getAllByRole('listitem')).toHaveLength(11);
    expect(within(cards[0]).getAllByRole('listitem')).toHaveLength(7);
    expect(cards[2]).toHaveTextContent('Kuruluşunuza özel fiyat');
  });

  it('every package leads to contact', () => {
    const { getAllByRole } = renderWithProviders(<PricingView />);
    const actions = getAllByRole('link', { name: /İletişime geçin/ });
    expect(actions).toHaveLength(3);
    for (const a of actions) expect(a.getAttribute('href')).toMatch(/^\/contact\?subject=/);
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<PricingView />);
    await expectNoAxeViolations(container);
  });
});
