import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Calendar, fromIsoDate, toIsoDate } from './_calendar';

describe('Calendar', () => {
  it('round-trips ISO dates without a timezone shift', () => {
    expect(toIsoDate(fromIsoDate('2026-03-29'))).toBe('2026-03-29');
    expect(toIsoDate(fromIsoDate('2026-10-25'))).toBe('2026-10-25');
  });

  it('labels navigation from the catalogue and has no axe violations', async () => {
    const { container, getByRole } = renderWithProviders(<Calendar mode="single" defaultMonth={fromIsoDate('2026-09-01')} />);
    expect(getByRole('button', { name: 'Sonraki ay' })).toBeInTheDocument();
    await expectNoAxeViolations(container);
  });
});
