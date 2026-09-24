import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Switch } from './switch';

describe('Switch', () => {
  it('space toggles', async () => {
    const onCheckedChange = vi.fn();
    const { getByRole, user } = renderWithProviders(<Switch label="Kenar çubuğu daraltılmış" checked={false} onCheckedChange={onCheckedChange} />);
    getByRole('switch', { name: 'Kenar çubuğu daraltılmış' }).focus();
    await user.keyboard(' ');
    expect(onCheckedChange).toHaveBeenCalledWith(true);
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Switch label="Bildirimler" checked onCheckedChange={() => {}} />);
    await expectNoAxeViolations(container);
  });
});
