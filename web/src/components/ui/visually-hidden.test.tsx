import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { VisuallyHidden } from './visually-hidden';

describe('VisuallyHidden', () => {
  it('stays in the accessibility tree', () => {
    const { getByRole } = renderWithProviders(
      <button type="button">
        <svg aria-hidden />
        <VisuallyHidden>Kapat</VisuallyHidden>
      </button>,
    );
    const name = getByRole('button', { name: 'Kapat' });
    expect(name.querySelector('span')).toHaveClass('sr-only');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<VisuallyHidden>Metin</VisuallyHidden>);
    await expectNoAxeViolations(container);
  });
});
