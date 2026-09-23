import { screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { EmailDialog } from './email-dialog';

const props = () => ({
  open: true, summary: { period: 'Ağustos 2026', building: 'Merkez' }, sending: false,
  onSend: vi.fn(), onClose: vi.fn(),
});

describe('EmailDialog', () => {
  it('shows the period and building before sending (§7.14)', () => {
    renderWithProviders(<EmailDialog {...props()} />);
    expect(screen.getByRole('dialog', { name: 'Raporu e-posta ile gönder' })).toBeVisible();
    expect(screen.getByText('Ağustos 2026')).toBeVisible();
    expect(screen.getByText('Merkez')).toBeVisible();
  });

  it('refuses an address without an @ next to the field', async () => {
    const p = props();
    const { user } = renderWithProviders(<EmailDialog {...p} />);
    await user.type(screen.getByRole('textbox', { name: /Alıcı e-posta adresi/ }), 'yonetici');
    await user.click(screen.getByRole('button', { name: 'Gönder' }));
    expect(p.onSend).not.toHaveBeenCalled();
    expect(screen.getByText('Geçerli bir e-posta adresi girin.')).toBeVisible();
  });

  it('sends a valid address', async () => {
    const p = props();
    const { user } = renderWithProviders(<EmailDialog {...p} />);
    await user.type(screen.getByRole('textbox', { name: /Alıcı e-posta adresi/ }), 'yonetici@firma.com.tr');
    await user.click(screen.getByRole('button', { name: 'Gönder' }));
    expect(p.onSend).toHaveBeenCalledWith('yonetici@firma.com.tr');
  });

  it('shows the server’s refusal on the field', () => {
    renderWithProviders(<EmailDialog {...props()} error="Geçerli bir e-posta adresi girin." />);
    expect(screen.getByText('Geçerli bir e-posta adresi girin.')).toBeVisible();
  });
});
