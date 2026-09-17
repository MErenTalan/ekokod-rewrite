import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Dialog } from './dialog';

function WithTrigger({ onOpenChange }: { onOpenChange: (o: boolean) => void }) {
  const [open, setOpen] = useState(false);
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o);
        setOpen(o);
      }}
      title="Tarifeyi sil"
      description="Bu işlem geri alınamaz."
      trigger={<button type="button">Sil</button>}
      footer={<button type="button">Onayla</button>}
    >
      <input aria-label="Onay metni" />
    </Dialog>
  );
}

function WithoutTrigger() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Aç
      </button>
      <Dialog open={open} onOpenChange={setOpen} title="Not ekle">
        <input aria-label="Not" />
      </Dialog>
    </>
  );
}

describe('Dialog', () => {
  it('traps focus, Escape closes, focus returns (trigger)', async () => {
    const onOpenChange = vi.fn();
    const { getByRole, findByRole, queryByRole, user } = renderWithProviders(<WithTrigger onOpenChange={onOpenChange} />);
    const trigger = getByRole('button', { name: 'Sil' });
    await user.click(trigger);
    const dialog = await findByRole('dialog', { name: 'Tarifeyi sil' });
    expect(dialog).toContainElement(document.activeElement as HTMLElement);
    for (let i = 0; i < 10; i++) await user.tab();
    expect(dialog).toContainElement(document.activeElement as HTMLElement);
    await user.keyboard('{Escape}');
    expect(onOpenChange).toHaveBeenLastCalledWith(false);
    expect(queryByRole('dialog')).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it('without a trigger, focus restores to the element that opened it', async () => {
    const { getByRole, findByRole, queryByRole, user } = renderWithProviders(<WithoutTrigger />);
    const opener = getByRole('button', { name: 'Aç' });
    await user.click(opener);
    await findByRole('dialog', { name: 'Not ekle' });
    await user.click(getByRole('button', { name: 'Kapat' }));
    expect(queryByRole('dialog')).not.toBeInTheDocument();
    expect(opener).toHaveFocus();
  });

  it('has no axe violations when open', async () => {
    const { baseElement, getByRole, findByRole, user } = renderWithProviders(<WithTrigger onOpenChange={() => {}} />);
    await user.click(getByRole('button', { name: 'Sil' }));
    await findByRole('dialog');
    await expectNoAxeViolations(baseElement);
  });
});
