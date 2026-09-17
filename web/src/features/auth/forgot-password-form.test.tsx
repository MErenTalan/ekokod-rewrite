import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { ForgotPasswordForm } from './forgot-password-form';

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => vi.unstubAllGlobals());

const reply = (status: number, body: unknown, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } });

const CONFIRMATION = 'Bu adrese kayıtlı bir hesap varsa şifre sıfırlama bağlantısı gönderildi.';

describe('ForgotPasswordForm', () => {
  it.each([
    ['a known address', reply(202, {})],
    ['an unknown address', reply(202, {})],
    ['a refused address', reply(422, { error: { code: 'validation_failed', message: 'x' } })],
  ])('confirms the same way for %s', async (_, response) => {
    fetchMock.mockResolvedValue(response);
    const ui = renderWithProviders(<ForgotPasswordForm />);
    await ui.user.type(ui.getByLabelText('E-posta'), 'kimse@firma.com.tr');
    await ui.user.click(ui.getByRole('button', { name: 'Bağlantı Gönder' }));
    expect(await ui.findByRole('status')).toHaveTextContent(CONFIRMATION);
    expect(await (fetchMock.mock.calls[0][0] as Request).json()).toEqual({ email: 'kimse@firma.com.tr' });
  });

  it('reports a rate limit instead of confirming', async () => {
    fetchMock.mockResolvedValue(reply(429, { error: { code: 'rate_limited', message: 'x' } }, { 'Retry-After': '60' }));
    const ui = renderWithProviders(<ForgotPasswordForm />);
    await ui.user.type(ui.getByLabelText('E-posta'), 'kimse@firma.com.tr');
    await ui.user.click(ui.getByRole('button', { name: 'Bağlantı Gönder' }));
    expect(await ui.findByRole('alert')).toHaveTextContent('1 dakika sonra');
  });

  it('validates the address first', async () => {
    const ui = renderWithProviders(<ForgotPasswordForm />);
    await ui.user.type(ui.getByLabelText('E-posta'), 'olmaz');
    await ui.user.click(ui.getByRole('button', { name: 'Bağlantı Gönder' }));
    expect(ui.getAllByText('Geçerli bir e-posta adresi girin.').length).toBeGreaterThan(0);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('has no axe violations', async () => {
    const ui = renderWithProviders(<ForgotPasswordForm />);
    await expectNoAxeViolations(ui.container);
  });
});
