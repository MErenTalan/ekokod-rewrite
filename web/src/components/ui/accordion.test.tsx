import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Accordion } from './accordion';

const items = [
  { value: 'a', title: 'Reaktif ceza nasıl hesaplanır?', content: 'Endüktif oran %20 sınırını aşarsa…' },
  { value: 'b', title: 'Fatura dönemi', content: 'Ayın ilk günü kapanır.' },
];

describe('Accordion', () => {
  it('headers are buttons that expand their panel', async () => {
    const { getByRole, user } = renderWithProviders(<Accordion items={items} type="single" />);
    const header = getByRole('button', { name: 'Reaktif ceza nasıl hesaplanır?' });
    expect(header).toHaveAttribute('aria-expanded', 'false');
    await user.click(header);
    expect(header).toHaveAttribute('aria-expanded', 'true');
    expect(getByRole('region', { name: 'Reaktif ceza nasıl hesaplanır?' })).toHaveTextContent('Endüktif oran');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Accordion items={items} type="multiple" />);
    await expectNoAxeViolations(container);
  });
});
