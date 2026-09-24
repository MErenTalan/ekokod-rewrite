import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Checkbox } from './checkbox';

describe('Checkbox', () => {
  it('clicking the label toggles it', async () => {
    const onCheckedChange = vi.fn();
    const { getByText, getByRole, user } = renderWithProviders(
      <Checkbox label="Yalnızca aktif analizörler" checked={false} onCheckedChange={onCheckedChange} />,
    );
    await user.click(getByText('Yalnızca aktif analizörler'));
    expect(onCheckedChange).toHaveBeenCalledWith(true);
    expect(getByRole('checkbox', { name: 'Yalnızca aktif analizörler' })).toHaveAttribute('aria-checked', 'false');
  });

  it('indeterminate is mixed', () => {
    const { getByRole } = renderWithProviders(<Checkbox label="Tümü" checked="indeterminate" onCheckedChange={() => {}} />);
    expect(getByRole('checkbox')).toHaveAttribute('aria-checked', 'mixed');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Checkbox label="Tümü" checked onCheckedChange={() => {}} error="Seçim zorunlu" />);
    await expectNoAxeViolations(container);
  });
});
