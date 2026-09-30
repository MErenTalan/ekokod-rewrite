import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { NumberInput } from './number-input';

function setup(value: string | null = null, extra: Partial<Parameters<typeof NumberInput>[0]> = {}) {
  const onValueChange = vi.fn();
  const utils = renderWithProviders(<NumberInput label="Tutar" value={value} onValueChange={onValueChange} {...extra} />);
  return { ...utils, onValueChange, input: utils.getByLabelText('Tutar') as HTMLInputElement };
}

describe('NumberInput', () => {
  it('parses Turkish input to a decimal string', async () => {
    const { input, user, onValueChange } = setup();
    await user.type(input, '1.234,56');
    await user.tab();
    expect(onValueChange).toHaveBeenCalledWith('1234.56');
  });

  it('keeps precision beyond float', async () => {
    const { input, user, onValueChange } = setup();
    await user.type(input, '12345678901234567,891');
    await user.tab();
    expect(onValueChange).toHaveBeenCalledWith('12345678901234567.891');
  });

  it('empty is null, not zero', async () => {
    const { input, user, onValueChange } = setup('12.5');
    expect(input.value).toBe('12,5');
    await user.clear(input);
    await user.tab();
    expect(onValueChange).toHaveBeenCalledWith(null);
  });

  it('rejects letters', async () => {
    const { input, user } = setup();
    await user.type(input, '12a');
    expect(input.value).toBe('12');
  });

  it('out-of-range input is flagged, not clamped or emitted', async () => {
    const { input, user, onValueChange } = setup(null, { max: '100' });
    await user.type(input, '100,01');
    await user.tab();
    expect(onValueChange).not.toHaveBeenCalled();
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(input.value).toBe('100,01');
  });

  it('reads a lone dot with other than three decimals as a decimal point', async () => {
    const { input, user, onValueChange } = setup(null, { min: '-90', max: '90' });
    await user.type(input, '41.0082');
    await user.tab();
    expect(onValueChange).toHaveBeenCalledWith('41.0082');
  });

  it('keeps 1.234 as Turkish thousands', async () => {
    const { input, user, onValueChange } = setup();
    await user.type(input, '1.234');
    await user.tab();
    expect(onValueChange).toHaveBeenCalledWith('1234');
  });

  it('shows an empty string value as empty, so it can be typed into', async () => {
    const { input, user, onValueChange } = setup('');
    expect(input.value).toBe('');
    await user.type(input, '100');
    await user.tab();
    expect(onValueChange).toHaveBeenCalledWith('100');
  });

  it('names a fraction in a whole-number field instead of sending it', async () => {
    const { input, user, onValueChange, getByText } = setup(null, { fractionDigits: 0 });
    await user.type(input, '12,5');
    await user.tab();
    expect(onValueChange).not.toHaveBeenCalled();
    expect(getByText('Tam sayı girin')).toBeVisible();
  });

  it('names an out-of-range value', async () => {
    const { input, user, getByText } = setup(null, { min: '-90', max: '90' });
    await user.type(input, '410082');
    await user.tab();
    expect(getByText('Aralık dışında')).toBeVisible();
  });

  it('has no axe violations', async () => {
    const { container } = setup('1234.5', { unit: 'kWh' });
    await expectNoAxeViolations(container);
  });
});
