import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { mockRouter } from '@/test/navigation';
import { renderWithProviders } from '@/test/render';

import { LoginForm } from './login-form';

const adopt = vi.hoisted(() => vi.fn(async () => {}));
vi.mock('./adopt-preferences', () => ({ adoptProfilePreferences: adopt }));

const me = {
  id: 'u1', name: 'Ayşe Yılmaz', email: 'ayse@firma.com.tr', role: 'company_admin', locale: 'en',
  ui_preferences: '%7B%22theme%22%3A%22dark%22%7D', permissions: ['nav.core'], session_id: 's1',
  company: { id: 'c1', name: 'Firma' },
};

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  mockRouter.replace.mockClear();
  adopt.mockClear();
});
afterEach(() => vi.unstubAllGlobals());

const reply = (status: number, body: unknown, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } });

async function fill(ui: ReturnType<typeof renderWithProviders>, remember = true) {
  await ui.user.type(ui.getByLabelText('E-posta'), 'ayse@firma.com.tr');
  await ui.user.type(ui.getByLabelText('Şifre'), 'Guclu!Parola-91');
  if (remember) await ui.user.click(ui.getByRole('checkbox', { name: 'Bu Cihazı Hatırla' }));
  await ui.user.click(ui.getByRole('button', { name: 'Giriş Yap' }));
}

describe('LoginForm', () => {
  it('shows the legacy labels and the forgot-password link', () => {
    const ui = renderWithProviders(<LoginForm />);
    expect(ui.getByLabelText('E-posta')).toHaveAttribute('type', 'email');
    expect(ui.getByLabelText('Şifre')).toHaveAttribute('type', 'password');
    expect(ui.getByRole('checkbox', { name: 'Bu Cihazı Hatırla' })).not.toBeChecked();
    expect(ui.getByRole('link', { name: 'Şifremi Unuttum' })).toHaveAttribute('href', '/auth/forgot-password');
  });

  it('posts the credentials, adopts the profile preferences and goes to next', async () => {
    fetchMock.mockResolvedValue(reply(200, me));
    const ui = renderWithProviders(<LoginForm next="/ekorm/consumption?month=2026-08" />);
    await fill(ui);
    await waitFor(() => expect(mockRouter.replace).toHaveBeenCalledWith('/ekorm/consumption?month=2026-08'));
    const request = fetchMock.mock.calls[0][0] as Request;
    expect(new URL(request.url).pathname).toBe('/api/v1/auth/login');
    expect(await request.json()).toEqual({ email: 'ayse@firma.com.tr', password: 'Guclu!Parola-91', remember_me: true });
    expect(adopt).toHaveBeenCalledWith(expect.objectContaining({ ui_preferences: me.ui_preferences, locale: 'en' }));
  });

  it.each(['//evil.com', 'https://evil.com/ekorm', '/auth/login', undefined])('ignores an unsafe next (%s)', async (next) => {
    fetchMock.mockResolvedValue(reply(200, me));
    const ui = renderWithProviders(<LoginForm next={next} />);
    await fill(ui, false);
    await waitFor(() => expect(mockRouter.replace).toHaveBeenCalledWith('/ekorm'));
  });

  it('announces wrong credentials and moves focus to the message', async () => {
    fetchMock.mockResolvedValue(reply(401, { error: { code: 'invalid_credentials', message: 'x' } }));
    const ui = renderWithProviders(<LoginForm />);
    await fill(ui);
    const alert = await ui.findByRole('alert');
    expect(alert).toHaveTextContent('E-posta veya şifre hatalı.');
    await waitFor(() => expect(alert.parentElement).toHaveFocus());
    expect(mockRouter.replace).not.toHaveBeenCalled();
  });

  it('tells a rate-limited user how many minutes to wait', async () => {
    fetchMock.mockResolvedValue(reply(429, { error: { code: 'rate_limited', message: 'x' } }, { 'Retry-After': '540' }));
    const ui = renderWithProviders(<LoginForm />);
    await fill(ui);
    expect(await ui.findByRole('alert')).toHaveTextContent('Çok fazla deneme yapıldı. 9 dakika sonra tekrar deneyin.');
  });

  it('validates empty fields before calling the API', async () => {
    const ui = renderWithProviders(<LoginForm />);
    await ui.user.click(ui.getByRole('button', { name: 'Giriş Yap' }));
    expect(await ui.findByText('Lütfen aşağıdaki alanları düzeltin')).toBeInTheDocument();
    expect(ui.getAllByText('E-posta adresinizi girin.').length).toBeGreaterThan(0);
    expect(ui.getAllByText('Şifrenizi girin.').length).toBeGreaterThan(0);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('explains a device mismatch', () => {
    const ui = renderWithProviders(<LoginForm reason="device_mismatch" />);
    expect(ui.getByRole('alert')).toHaveTextContent(
      'Oturumunuz başka bir cihazda kullanıldığı için sonlandırıldı. Lütfen tekrar giriş yapın.',
    );
  });

  it('confirms a password reset', () => {
    const ui = renderWithProviders(<LoginForm reason="password_reset" />);
    expect(ui.getByRole('status')).toHaveTextContent('Şifreniz güncellendi. Yeni şifrenizle giriş yapın.');
  });

  it('has no axe violations', async () => {
    const ui = renderWithProviders(<LoginForm reason="device_mismatch" />);
    await expectNoAxeViolations(ui.container);
  });
});
