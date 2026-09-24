import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { useToast } from './toast';

function Trigger() {
  const { toast } = useToast();
  return (
    <button type="button" onClick={() => toast({ tone: 'success', title: 'Kaydedildi', description: 'Tarife güncellendi' })}>
      Kaydet
    </button>
  );
}

describe('Toast', () => {
  it('toast text reaches a live region', async () => {
    const { getByRole, baseElement, user } = renderWithProviders(<Trigger />);
    await user.click(getByRole('button', { name: 'Kaydet' }));
    // Radix announces a copy of the toast text through its own role=status live region.
    await expect
      .poll(() => [...baseElement.querySelectorAll('[role="status"][aria-live]')].some((el) => el.textContent?.includes('Kaydedildi')))
      .toBe(true);
  });

  it('has no axe violations when shown', async () => {
    const { baseElement, getByRole, findAllByText, user } = renderWithProviders(<Trigger />);
    await user.click(getByRole('button', { name: 'Kaydet' }));
    await findAllByText('Kaydedildi');
    await expectNoAxeViolations(baseElement);
  });
});
