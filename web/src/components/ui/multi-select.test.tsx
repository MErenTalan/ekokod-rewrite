import { within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { MultiSelect } from './multi-select';

const options = ['Merkez Bina', 'Depo', 'Ofis', 'Fabrika', 'GES Sahası'].map((label, i) => ({ value: `b${i}`, label }));

describe('MultiSelect', () => {
  it('toggles values and summarises overflow', async () => {
    const onValueChange = vi.fn();
    const { getByRole, findByRole, rerender, user } = renderWithProviders(
      <MultiSelect label="Binalar" options={options} value={[]} onValueChange={onValueChange} searchPlaceholder="Bina ara" emptyText="Yok" maxChips={2} />,
    );
    await user.click(getByRole('combobox', { name: 'Binalar' }));
    await user.click(await findByRole('option', { name: 'Depo' }));
    expect(onValueChange).toHaveBeenLastCalledWith(['b1']);
    rerender(
      <MultiSelect label="Binalar" options={options} value={['b0', 'b1', 'b2', 'b3']} onValueChange={onValueChange} searchPlaceholder="Bina ara" emptyText="Yok" maxChips={2} />,
    );
    const trigger = getByRole('combobox', { name: 'Binalar' });
    expect(within(trigger).getByText('Merkez Bina')).toBeInTheDocument();
    expect(within(trigger).getByText('+2')).toBeInTheDocument();
    await user.click(await findByRole('option', { name: 'Depo' }));
    expect(onValueChange).toHaveBeenLastCalledWith(['b0', 'b2', 'b3']);
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(
      <MultiSelect label="Binalar" options={options} value={['b0']} onValueChange={() => {}} searchPlaceholder="Bina ara" emptyText="Yok" />,
    );
    await expectNoAxeViolations(container);
  });
});
