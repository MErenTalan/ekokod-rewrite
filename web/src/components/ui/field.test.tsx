import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Field } from './field';

describe('Field', () => {
  it('label, description and error are wired', () => {
    const { getByLabelText, getByText } = renderWithProviders(
      <Field label="E-posta" description="İş adresiniz" error="Geçerli bir e-posta girin">
        {({ controlId, describedBy, invalid }) => (
          <input id={controlId} aria-describedby={describedBy} aria-invalid={invalid || undefined} />
        )}
      </Field>,
    );
    const input = getByLabelText('E-posta');
    const ids = input.getAttribute('aria-describedby')!.split(' ');
    expect(ids).toContain(getByText('İş adresiniz').id);
    expect(ids).toContain(getByText('Geçerli bir e-posta girin').closest('p')!.id);
    expect(input).toHaveAttribute('aria-invalid', 'true');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(
      <Field label="E-posta" required error="Zorunlu alan">
        {({ controlId, describedBy, invalid }) => <input id={controlId} aria-describedby={describedBy} aria-invalid={invalid} />}
      </Field>,
    );
    await expectNoAxeViolations(container);
  });
});
