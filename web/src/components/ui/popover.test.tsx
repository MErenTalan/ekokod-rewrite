import { within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Popover } from './popover';

describe('Popover', () => {
  it('opens from its trigger, Escape closes and returns focus', async () => {
    const { getByRole, findByRole, queryByRole, user } = renderWithProviders(
      <Popover trigger={<button type="button">Filtre</button>} label="Filtre seçenekleri">
        <button type="button">Uygula</button>
      </Popover>,
    );
    const trigger = getByRole('button', { name: 'Filtre' });
    await user.click(trigger);
    const dialog = await findByRole('dialog', { name: 'Filtre seçenekleri' });
    expect(within(dialog).getByRole('button', { name: 'Uygula' })).toBeInTheDocument();
    expect(trigger).toHaveAttribute('aria-expanded', 'true');
    await user.keyboard('{Escape}');
    expect(queryByRole('dialog')).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it('has no axe violations when open', async () => {
    const { baseElement, getByRole, findByRole, user } = renderWithProviders(
      <Popover trigger={<button type="button">Filtre</button>} label="Filtre seçenekleri">
        <p>İçerik</p>
      </Popover>,
    );
    await user.click(getByRole('button'));
    await findByRole('dialog');
    await expectNoAxeViolations(baseElement);
  });
});
