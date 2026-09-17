import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { FormErrorSummary } from './form-error-summary';

const errors = [{ fieldId: 'email', message: 'E-posta zorunlu' }];

describe('FormErrorSummary', () => {
  it('focuses and links to fields', () => {
    const { rerender, getByRole, queryByRole } = renderWithProviders(<FormErrorSummary title="Formu düzeltin" errors={[]} />);
    expect(queryByRole('alert')).not.toBeInTheDocument();
    rerender(<FormErrorSummary title="Formu düzeltin" errors={errors} />);
    expect(getByRole('alert')).toHaveFocus();
    expect(getByRole('link', { name: 'E-posta zorunlu' })).toHaveAttribute('href', '#email');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<FormErrorSummary title="Formu düzeltin" errors={errors} />);
    await expectNoAxeViolations(container);
  });
});
