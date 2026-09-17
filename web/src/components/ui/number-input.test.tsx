import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { compareDecimal, NumberInput } from './number-input';

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

  it('compares decimals without floats', () => {
    expect(compareDecimal('12345678901234567.891', '12345678901234567.89')).toBe(1);
    expect(compareDecimal('-2', '-10')).toBe(1);
    expect(compareDecimal('0.10', '0.1')).toBe(0);
  });

  it('has no axe violations', async () => {
    const { container } = setup('1234.5', { unit: 'kWh' });
    await expectNoAxeViolations(container);
  });
});
