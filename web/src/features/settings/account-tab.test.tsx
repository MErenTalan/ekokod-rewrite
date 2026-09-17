import { describe, expect, it, vi } from 'vitest';

import type { Me, Session } from '@/lib/api/types';
import fixture from '@/lib/session/permissions.fixture.json';
import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { AccountTabView } from './account-tab';

const profile: Me = {
  id: 'u-1',
  name: 'Ayşe Kaya',
  email: 'ayse@ornek.com.tr',
  phone: '0532 000 00 00',
  role: 'company_admin',
  locale: 'tr',
  permissions: fixture.company_admin as Me['permissions'],
  session_id: 's-1',
  company: { id: 'c-1', name: 'Anadolu Tekstil' },
};

const sessions: Session[] = [
  {
    id: 's-1',
    client: 'web',
    user_agent: 'Chrome · Windows',
    ip: '203.0.113.7',
    created_at: '2026-09-10T09:00:00+03:00',
    last_used_at: '2026-09-17T08:00:00+03:00',
    expires_at: '2026-10-17T09:00:00+03:00',
    current: true,
  },
  {
    id: 's-2',
    client: 'mobile',
    user_agent: 'ekokod iOS',
    ip: '203.0.113.9',
    created_at: '2026-09-01T09:00:00+03:00',
    last_used_at: null,
    expires_at: '2026-10-01T09:00:00+03:00',
    current: false,
  },
];

const view = (overrides: Partial<React.ComponentProps<typeof AccountTabView>> = {}) => (
  <AccountTabView
    profile={profile}
    readOnly={false}
    sessions={sessions}
    onSaveProfile={() => {}}
    onChangePassword={() => {}}
    onRevokeSession={() => {}}
    onLogoutAll={() => {}}
    {...overrides}
  />
);

describe('AccountTabView', () => {
  it('saves the personal details', async () => {
    const onSaveProfile = vi.fn();
    const r = renderWithProviders(view({ onSaveProfile }));
    await r.user.clear(r.getByLabelText('Telefon'));
    await r.user.type(r.getByLabelText('Telefon'), '0533 111 11 11');
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    expect(onSaveProfile).toHaveBeenCalledWith({
      name: 'Ayşe Kaya',
      email: 'ayse@ornek.com.tr',
      phone: '0533 111 11 11',
    });
  });

  it('changes the password with the rules in sight', async () => {
    const onChangePassword = vi.fn();
    const r = renderWithProviders(view({ onChangePassword }));
    await r.user.type(r.getByLabelText(/Mevcut şifre/), 'Eski!Sifre-42');
    await r.user.type(r.getByLabelText(/Yeni şifre/), 'Guvenli!Sifre-43');
    expect(r.getByText(/En az 10 karakter/)).toBeInTheDocument();
    await r.user.click(r.getByRole('button', { name: 'Şifreyi değiştir' }));
    expect(onChangePassword).toHaveBeenCalledWith({ current_password: 'Eski!Sifre-42', new_password: 'Guvenli!Sifre-43' });
  });

  it('shows a wrong current password next to its field', () => {
    const r = renderWithProviders(view({ fieldErrors: { current_password: 'Geçersiz değer' } }));
    expect(r.getByText('Geçersiz değer')).toBeInTheDocument();
  });

  it('lists the sessions and cannot revoke the current one', async () => {
    const onRevokeSession = vi.fn();
    const r = renderWithProviders(view({ onRevokeSession }));
    expect(r.getByRole('row', { name: /Chrome · Windows/ })).toHaveTextContent('Bu cihaz');
    const revokes = r.getAllByRole('button', { name: 'Oturumu kapat' });
    expect(revokes).toHaveLength(1);
    await r.user.click(revokes[0]);
    expect(onRevokeSession).toHaveBeenCalledWith('s-2');
  });

  it('is read-only for the shared demo account', () => {
    const r = renderWithProviders(view({ readOnly: true, profile: { ...profile, role: 'demo' } }));
    expect(r.getByText('Demo hesabı paylaşımlıdır; profil ve şifre değiştirilemez.')).toBeInTheDocument();
    expect(r.getByLabelText(/Ad soyad/)).toBeDisabled();
    expect(r.queryByRole('button', { name: 'Kaydet' })).toBeNull();
    expect(r.queryByText('Aktif oturumlar')).toBeNull();
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(view());
    await expectNoAxeViolations(r.container);
  });
});
