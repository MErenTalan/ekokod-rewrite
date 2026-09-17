import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Input } from './input';

describe('Input', () => {
  it('is named by its visible label and reports errors', () => {
    const { getByLabelText } = renderWithProviders(<Input label="Bina adı" error="Bina adı zorunlu" defaultValue="" />);
    expect(getByLabelText('Bina adı')).toHaveAttribute('aria-invalid', 'true');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Input label="Bina adı" description="Raporlarda görünür" />);
    await expectNoAxeViolations(container);
  });
});
