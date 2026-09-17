import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Select } from './select';

const options = [
  { value: 'a', label: 'Saatlik' },
  { value: 'b', label: 'Günlük' },
  { value: 'c', label: 'Aylık' },
];

describe('Select', () => {
  it('keyboard selects an option', async () => {
    const onValueChange = vi.fn();
    const { getByRole, user } = renderWithProviders(
      <Select label="Çözünürlük" options={options} value={null} onValueChange={onValueChange} placeholder="Seçin" />,
    );
    getByRole('combobox', { name: 'Çözünürlük' }).focus();
    await user.keyboard('{ArrowDown}{ArrowDown}{Enter}');
    expect(onValueChange).toHaveBeenCalledWith('b');
  });

  it('shows the selected label', () => {
    const { getByRole } = renderWithProviders(<Select label="Çözünürlük" options={options} value="c" onValueChange={() => {}} />);
    expect(getByRole('combobox', { name: 'Çözünürlük' })).toHaveTextContent('Aylık');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Select label="Çözünürlük" options={options} value="a" onValueChange={() => {}} error="Zorunlu" />);
    await expectNoAxeViolations(container);
  });
});
