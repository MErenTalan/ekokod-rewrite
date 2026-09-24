import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { LeadForm, type LeadFormProps } from './lead-form';

const props = (over: Partial<LeadFormProps> = {}): LeadFormProps => ({ kind: 'contact', errors: {}, status: 'idle', sending: false, onSubmit: vi.fn(), onReset: vi.fn(), ...over });

describe('LeadForm', () => {
  it('hides the honeypot from people and assistive technology', () => {
    const { container, queryByLabelText } = renderWithProviders(<LeadForm {...props()} />);
    const trap = container.querySelector('input[name="website"]');
    expect(trap).toHaveAttribute('tabindex', '-1');
    expect(trap).toHaveAttribute('autocomplete', 'off');
    expect(trap?.closest('[aria-hidden="true"]')).not.toBeNull();
    expect(queryByLabelText('Bu alanı boş bırakın')).toBeNull();
  });

  it('marks the demo form fields it requires', () => {
    const { getByLabelText } = renderWithProviders(<LeadForm {...props({ kind: 'demo' })} />);
    expect(getByLabelText(/Telefon/)).toBeRequired();
    expect(getByLabelText(/Şirket/)).toBeRequired();
    expect(getByLabelText(/Mesajınız/)).not.toBeRequired();
  });

  it('replaces the form with a confirmation that offers another message', async () => {
    const onReset = vi.fn();
    const { getByRole, queryByRole, user } = renderWithProviders(<LeadForm {...props({ status: 'sent', onReset })} />);
    expect(getByRole('status')).toHaveTextContent('Mesajınız iletildi');
    expect(queryByRole('button', { name: 'Gönder' })).toBeNull();
    await user.click(getByRole('button', { name: 'Yeni mesaj gönder' }));
    expect(onReset).toHaveBeenCalled();
  });

  it('has no axe violations, with errors shown', async () => {
    const { container } = renderWithProviders(<LeadForm {...props({ errors: { email: 'Geçerli bir e-posta girin' }, status: 'failed' })} />);
    await expectNoAxeViolations(container);
  });
});
