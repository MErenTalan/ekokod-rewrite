import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { DateRangePicker } from './date-range-picker';

const presets = [{ id: 'last7', label: 'Son 7 gün', range: { from: '2026-09-11', to: '2026-09-17' } }];

describe('DateRangePicker', () => {
  it('preset applies its range', async () => {
    const onValueChange = vi.fn();
    const { getByRole, findByRole, user } = renderWithProviders(
      <DateRangePicker label="Dönem" value={null} onValueChange={onValueChange} presets={presets} />,
    );
    await user.click(getByRole('button', { name: /Dönem/ }));
    await user.click(await findByRole('button', { name: 'Son 7 gün' }));
    expect(onValueChange).toHaveBeenCalledWith({ from: '2026-09-11', to: '2026-09-17' });
  });

  it('shows the range in the trigger', () => {
    const { getByRole } = renderWithProviders(
      <DateRangePicker label="Dönem" value={{ from: '2026-09-11', to: '2026-09-17' }} onValueChange={() => {}} />,
    );
    expect(getByRole('button', { name: /Dönem/ })).toHaveTextContent('11 Eyl 2026 – 17 Eyl 2026');
  });

  it('picking two days emits an ISO range', async () => {
    const onValueChange = vi.fn();
    const { getByRole, findAllByRole, user } = renderWithProviders(
      <DateRangePicker label="Dönem" value={{ from: '2026-09-01', to: '2026-09-02' }} onValueChange={onValueChange} />,
    );
    await user.click(getByRole('button', { name: /Dönem/ }));
    const [september] = await findAllByRole('grid');
    const day = (n: string) => [...september.querySelectorAll('button')].find((b) => b.textContent === n)!;
    await user.click(day('10'));
    await user.click(day('12'));
    expect(onValueChange).toHaveBeenLastCalledWith({ from: '2026-09-01', to: '2026-09-12' });
  });

  it('has no axe violations when open', async () => {
    const { baseElement, getByRole, findAllByRole, user } = renderWithProviders(
      <DateRangePicker label="Dönem" value={null} onValueChange={() => {}} presets={presets} />,
    );
    await user.click(getByRole('button', { name: /Dönem/ }));
    await findAllByRole('grid');
    await expectNoAxeViolations(baseElement);
  });
});
