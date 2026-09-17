import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Textarea } from './textarea';

describe('Textarea', () => {
  it('defaults to four rows', () => {
    const { getByLabelText } = renderWithProviders(<Textarea label="Not" />);
    expect(getByLabelText('Not')).toHaveAttribute('rows', '4');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Textarea label="Not" error="En fazla 500 karakter" />);
    await expectNoAxeViolations(container);
  });
});
