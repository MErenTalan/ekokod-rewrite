import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { mockRouter } from '@/test/navigation';
import { renderWithProviders } from '@/test/render';

import { ResetPasswordForm } from './reset-password-form';

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  mockRouter.replace.mockClear();
});
afterEach(() => vi.unstubAllGlobals());

const reply = (status: number, body?: unknown) =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

async function submit(ui: ReturnType<typeof renderWithProviders>, password: string, confirm = password) {
  await ui.user.type(ui.getByLabelText('Yeni Şifre'), password);
  await ui.user.type(ui.getByLabelText('Yeni Şifre Onayı'), confirm);
  await ui.user.click(ui.getByRole('button', { name: 'Şifreyi Güncelle' }));
}

describe('ResetPasswordForm', () => {
  it('sets the password and returns to login with the reason', async () => {
    fetchMock.mockResolvedValue(reply(204));
    const ui = renderWithProviders(<ResetPasswordForm token="tok-1" />);
    await submit(ui, 'Guvenli!Sifre-42');
    await waitFor(() => expect(mockRouter.replace).toHaveBeenCalledWith('/auth/login?reason=password_reset'));
    const request = fetchMock.mock.calls[0][0] as Request;
    expect(await request.json()).toEqual({ token: 'tok-1', password: 'Guvenli!Sifre-42' });
  });

  it('refuses a password that breaks a client rule without calling the API', async () => {
    const ui = renderWithProviders(<ResetPasswordForm token="tok-1" />);
    await submit(ui, 'kisa');
    expect(ui.getAllByText('Şifre en az 10 karakter olmalıdır.').length).toBeGreaterThan(0);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('refuses mismatched confirmation', async () => {
    const ui = renderWithProviders(<ResetPasswordForm token="tok-1" />);
    await submit(ui, 'Guvenli!Sifre-42', 'Guvenli!Sifre-43');
    expect(ui.getAllByText('Şifreler eşleşmiyor.').length).toBeGreaterThan(0);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('maps server policy codes to messages', async () => {
    fetchMock.mockResolvedValue(
      reply(422, { error: { code: 'validation_failed', message: 'x', details: { password: ['password_personal', 'password_reused'] } } }),
    );
    const ui = renderWithProviders(<ResetPasswordForm token="tok-1" />);
    await submit(ui, 'Guvenli!Sifre-42');
    expect(
      (await ui.findAllByText('Şifreniz adınızın, e-posta adresinizin veya şirket adınızın parçalarını içermemelidir. Yeni şifre, son kullandığınız şifrelerden biri olamaz.')).length,
    ).toBeGreaterThan(0);
    expect(mockRouter.replace).not.toHaveBeenCalled();
  });

  it('offers a new link when the token is spent', async () => {
    fetchMock.mockResolvedValue(reply(400, { error: { code: 'reset_token_invalid', message: 'x' } }));
    const ui = renderWithProviders(<ResetPasswordForm token="tok-1" />);
    await submit(ui, 'Guvenli!Sifre-42');
    expect(await ui.findByRole('alert')).toHaveTextContent('Bu bağlantının süresi dolmuş veya bağlantı daha önce kullanılmış.');
    expect(ui.getByRole('link', { name: 'Yeni Bağlantı İste' })).toHaveAttribute('href', '/auth/forgot-password');
  });

  it('explains a missing token', () => {
    const ui = renderWithProviders(<ResetPasswordForm />);
    expect(ui.getByRole('alert')).toHaveTextContent('Şifre sıfırlama bağlantısı eksik.');
  });

  it('has no axe violations', async () => {
    const ui = renderWithProviders(<ResetPasswordForm token="tok-1" />);
    await expectNoAxeViolations(ui.container);
  });
});
