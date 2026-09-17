import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Tabs } from './tabs';

const items = [
  { value: 'daily', label: 'Günlük', content: <p>Günlük tablo</p> },
  { value: 'monthly', label: 'Aylık', content: <p>Aylık tablo</p> },
  { value: 'yearly', label: 'Yıllık', content: <p>Yıllık tablo</p>, disabled: true },
];

describe('Tabs', () => {
  it('arrow keys move between tabs', async () => {
    const { getByRole, user } = renderWithProviders(<Tabs items={items} defaultValue="daily" />);
    getByRole('tab', { name: 'Günlük' }).focus();
    await user.keyboard('{ArrowRight}');
    expect(getByRole('tab', { name: 'Aylık' })).toHaveAttribute('aria-selected', 'true');
    expect(getByRole('tabpanel')).toHaveTextContent('Aylık tablo');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Tabs items={items} defaultValue="daily" />);
    await expectNoAxeViolations(container);
  });
});
