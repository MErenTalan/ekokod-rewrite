import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { RadioGroup } from './radio-group';

const options = [
  { value: 'first', label: 'Dikey' },
  { value: 'second', label: 'Yatay' },
];

describe('RadioGroup', () => {
  it('arrow keys move selection', async () => {
    const onValueChange = vi.fn();
    const { getByRole, user } = renderWithProviders(
      <RadioGroup label="Yerleşim" options={options} value="first" onValueChange={onValueChange} orientation="horizontal" />,
    );
    expect(getByRole('group', { name: 'Yerleşim' })).toBeInTheDocument();
    getByRole('radio', { name: 'Dikey' }).focus();
    // Radix checks on focus only while the arrow key is still held (it focuses in a timeout).
    await user.keyboard('{ArrowRight>}{/ArrowRight}');
    expect(onValueChange).toHaveBeenCalledWith('second');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<RadioGroup label="Yerleşim" options={options} value="first" onValueChange={() => {}} />);
    await expectNoAxeViolations(container);
  });
});
