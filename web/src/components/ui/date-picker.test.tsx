import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { DatePicker } from './date-picker';

describe('DatePicker', () => {
  it('emits ISO dates and never a Date', async () => {
    const onValueChange = vi.fn();
    const { getByRole, findByRole, user } = renderWithProviders(<DatePicker label="Başlangıç" value="2026-09-10" onValueChange={onValueChange} />);
    const trigger = getByRole('button', { name: /Başlangıç/ });
    expect(trigger).toHaveTextContent('10 Eyl 2026');
    await user.click(trigger);
    const grid = await findByRole('grid');
    await user.click(grid.querySelector('[data-day="2026-09-15"] button')!);
    expect(onValueChange).toHaveBeenCalledWith('2026-09-15');
    expect(typeof onValueChange.mock.calls[0][0]).toBe('string');
  });

  it.each([
    ['tr', 'Pazartesi'],
    ['en', 'Monday'],
  ] as const)('week starts on Monday in %s', async (locale, monday) => {
    const { getByRole, findByRole, user } = renderWithProviders(<DatePicker label="Tarih" value="2026-09-10" onValueChange={() => {}} />, { locale });
    await user.click(getByRole('button', { name: /Tarih/ }));
    const grid = await findByRole('grid');
    // react-day-picker hides the short-name header row from AT; each th carries the full weekday (plan I-12).
    expect(grid.querySelector('th')).toHaveAttribute('aria-label', monday);
  });

  it('days outside min and max are disabled', async () => {
    const { getByRole, findByRole, user } = renderWithProviders(
      <DatePicker label="Tarih" value="2026-09-10" min="2026-09-05" max="2026-09-20" onValueChange={() => {}} />,
    );
    await user.click(getByRole('button', { name: /Tarih/ }));
    const grid = await findByRole('grid');
    expect(grid.querySelector('[data-day="2026-09-04"] button')).toBeDisabled();
    expect(grid.querySelector('[data-day="2026-09-05"] button')).toBeEnabled();
    expect(grid.querySelector('[data-day="2026-09-21"] button')).toBeDisabled();
  });

  it('has no axe violations when open', async () => {
    const { baseElement, getByRole, findByRole, user } = renderWithProviders(<DatePicker label="Tarih" value={null} onValueChange={() => {}} />);
    await user.click(getByRole('button', { name: /Tarih/ }));
    await findByRole('grid');
    await expectNoAxeViolations(baseElement);
  });
});
