import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { useAnnounce } from './live-announcer';

function Announcer({ message, politeness }: { message: string; politeness?: 'polite' | 'assertive' }) {
  const announce = useAnnounce();
  return (
    <button type="button" onClick={() => announce(message, politeness)}>
      Duyur
    </button>
  );
}

describe('LiveAnnouncer', () => {
  it('assertive uses the alert region, polite the status region', async () => {
    const { getByRole, user, baseElement, rerender } = renderWithProviders(<Announcer message="Alarm: sınır aşıldı" politeness="assertive" />);
    const region = (p: string) => baseElement.querySelector(`[aria-live="${p}"][aria-atomic="true"]`)?.textContent;
    await user.click(getByRole('button'));
    await expect.poll(() => region('assertive')).toBe('Alarm: sınır aşıldı');
    expect(region('polite')).toBe('');
    rerender(<Announcer message="Rapor hazır" />);
    await user.click(getByRole('button'));
    await expect.poll(() => region('polite')).toBe('Rapor hazır');
    expect(region('assertive')).toBe('Alarm: sınır aşıldı');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Announcer message="x" />);
    await expectNoAxeViolations(container);
  });
});
