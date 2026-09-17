import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { MonthPicker } from './month-picker';

describe('MonthPicker', () => {
  it('emits YYYY-MM and respects min', async () => {
    const onValueChange = vi.fn();
    const { getByRole, findByRole, user } = renderWithProviders(
      <MonthPicker label="Fatura dönemi" value="2026-05" min="2026-02" onValueChange={onValueChange} />,
    );
    const trigger = getByRole('button', { name: /Fatura dönemi/ });
    expect(trigger).toHaveTextContent('Mayıs 2026');
    await user.click(trigger);
    const jan = await findByRole('button', { name: 'Ocak 2026' });
    expect(jan).toHaveAttribute('aria-disabled', 'true');
    await user.click(jan);
    expect(onValueChange).not.toHaveBeenCalled();
    await user.click(getByRole('button', { name: 'Mart 2026' }));
    expect(onValueChange).toHaveBeenCalledWith('2026-03');
  });

  it('year arrows change the year', async () => {
    const { getByRole, findByRole, user } = renderWithProviders(<MonthPicker label="Dönem" value="2026-05" onValueChange={() => {}} />);
    await user.click(getByRole('button', { name: /Dönem/ }));
    await user.click(await findByRole('button', { name: 'Önceki yıl' }));
    expect(getByRole('button', { name: 'Mayıs 2025' })).toBeInTheDocument();
  });

  it('has no axe violations when open', async () => {
    const { baseElement, getByRole, findByRole, user } = renderWithProviders(<MonthPicker label="Dönem" value={null} onValueChange={() => {}} />);
    await user.click(getByRole('button', { name: /Dönem/ }));
    await findByRole('button', { name: 'Önceki yıl' });
    await expectNoAxeViolations(baseElement);
  });
});
