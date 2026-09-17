import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { useAnnounce } from './live-announcer';

function Announcer({ politeness }: { politeness?: 'polite' | 'assertive' }) {
  const announce = useAnnounce();
  return (
    <button type="button" onClick={() => announce('Rapor hazır', politeness)}>
      Duyur
    </button>
  );
}

describe('LiveAnnouncer', () => {
  it('assertive uses the alert region, polite the status region', async () => {
    const { getByRole, user, baseElement, rerender } = renderWithProviders(<Announcer politeness="assertive" />);
    await user.click(getByRole('button'));
    await expect.poll(() => baseElement.querySelector('[aria-live="assertive"]')?.textContent).toBe('Rapor hazır');
    rerender(<Announcer />);
    await user.click(getByRole('button'));
    await expect.poll(() => baseElement.querySelector('[aria-live="polite"]')?.textContent).toBe('Rapor hazır');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Announcer />);
    await expectNoAxeViolations(container);
  });
});
