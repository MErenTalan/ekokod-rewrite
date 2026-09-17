import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Slider } from './slider';

describe('Slider', () => {
  it('is labelled, steps with arrows and exposes the formatted value', async () => {
    const onValueChange = vi.fn();
    const { getByRole, getByText, user } = renderWithProviders(
      <Slider label="Köşe yuvarlaklığı" value={1} min={0.5} max={1.5} step={0.25} onValueChange={onValueChange} formatValue={(v) => `${v}×`} />,
    );
    const thumb = getByRole('slider', { name: 'Köşe yuvarlaklığı' });
    expect(thumb).toHaveAttribute('aria-valuetext', '1×');
    expect(getByText('1×')).toBeInTheDocument();
    thumb.focus();
    await user.keyboard('{ArrowRight}');
    expect(onValueChange).toHaveBeenCalledWith(1.25);
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Slider label="Köşe" value={1} min={0} max={2} step={1} onValueChange={() => {}} />);
    await expectNoAxeViolations(container);
  });
});
