import { within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { SMTPSettings } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { SmtpTabView } from './smtp-tab';

const settings: SMTPSettings = {
  host: 'smtp.ornek.com.tr',
  port: 465,
  secure: true,
  username: 'bildirim@ornek.com.tr',
  from_address: 'bildirim@ornek.com.tr',
  has_password: true,
  updated_at: '2026-09-01T09:00:00+03:00',
};

describe('SmtpTabView', () => {
  it('keeps the stored password when the field is left empty (R172)', async () => {
    const onSave = vi.fn();
    const r = renderWithProviders(<SmtpTabView settings={settings} onSave={onSave} onTest={() => {}} />);
    expect(r.getByText('Kayıtlı şifre korunur; değiştirmek için yazın')).toBeInTheDocument();
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    expect(onSave).toHaveBeenCalledWith({
      host: 'smtp.ornek.com.tr',
      port: 465,
      secure: true,
      username: 'bildirim@ornek.com.tr',
      from_address: 'bildirim@ornek.com.tr',
    });
  });

  it('sends a typed password when one is given', async () => {
    const onSave = vi.fn();
    const r = renderWithProviders(<SmtpTabView settings={settings} onSave={onSave} onTest={() => {}} />);
    await r.user.type(r.getByLabelText(/Şifre/), 'Yeni!Smtp-42');
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    expect(onSave.mock.calls[0][0]).toMatchObject({ password: 'Yeni!Smtp-42' });
  });

  it('asks for a recipient before sending a test', async () => {
    const onTest = vi.fn();
    const r = renderWithProviders(<SmtpTabView settings={settings} onSave={() => {}} onTest={onTest} />);
    await r.user.click(r.getByRole('button', { name: 'Test e-postası gönder' }));
    const dialog = await r.findByRole('dialog');
    await r.user.type(r.getByLabelText(/Alıcı adresi/), 'test@ornek.com.tr');
    await r.user.click(within(dialog).getByRole('button', { name: 'Test e-postası gönder' }));
    expect(onTest).toHaveBeenCalledWith('test@ornek.com.tr');
  });

  it('cannot send a test before any settings exist', () => {
    const r = renderWithProviders(<SmtpTabView settings={null} onSave={() => {}} onTest={() => {}} />);
    expect(r.getByRole('button', { name: 'Test e-postası gönder' })).toBeDisabled();
  });
});
