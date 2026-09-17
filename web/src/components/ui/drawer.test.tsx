import { useState } from 'react';
import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Drawer } from './drawer';

function Example() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Filtreler
      </button>
      <Drawer open={open} onOpenChange={setOpen} title="Filtreler" side="bottom">
        <p>Filtre alanları</p>
      </Drawer>
    </>
  );
}

describe('Drawer', () => {
  it('has the dialog role and a title', async () => {
    const { getByRole, findByRole, user } = renderWithProviders(<Example />);
    await user.click(getByRole('button', { name: 'Filtreler' }));
    expect(await findByRole('dialog', { name: 'Filtreler' })).toHaveAttribute('data-side', 'bottom');
  });

  it('without a trigger, focus restores on close', async () => {
    const { getByRole, findByRole, queryByRole, user } = renderWithProviders(<Example />);
    const opener = getByRole('button', { name: 'Filtreler' });
    await user.click(opener);
    await findByRole('dialog');
    await user.keyboard('{Escape}');
    expect(queryByRole('dialog')).not.toBeInTheDocument();
    expect(opener).toHaveFocus();
  });

  it('has no axe violations when open', async () => {
    const { baseElement, getByRole, findByRole, user } = renderWithProviders(<Example />);
    await user.click(getByRole('button', { name: 'Filtreler' }));
    await findByRole('dialog');
    await expectNoAxeViolations(baseElement);
  });
});
