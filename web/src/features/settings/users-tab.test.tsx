import { describe, expect, it, vi } from 'vitest';

import type { User } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { UsersTabView } from './users-tab';

const users = [
  { id: 'u-1', name: 'Ayşe Kaya', email: 'ayse@ornek.com.tr', role: 'company_admin', is_active: true, locale: 'tr', created_at: '', last_login_at: '2026-09-17T08:00:00+03:00' },
  { id: 'u-2', name: 'Can Demir', email: 'can@ornek.com.tr', role: 'company_readonly_admin', is_active: false, locale: 'tr', created_at: '', last_login_at: null },
] as User[];

describe('UsersTabView', () => {
  it('names the role and says when someone never signed in', () => {
    const r = renderWithProviders(
      <UsersTabView users={users} selfId="u-1" canEdit onAdd={() => {}} onEdit={() => {}} onDelete={() => {}} />,
    );
    expect(r.getByRole('row', { name: /Ayşe Kaya/ })).toHaveTextContent('Şirket Yöneticisi');
    expect(r.getByRole('row', { name: /Can Demir/ })).toHaveTextContent('Hiç');
  });

  it('cannot delete the caller own row (R174)', () => {
    const r = renderWithProviders(
      <UsersTabView users={users} selfId="u-1" canEdit onAdd={() => {}} onEdit={() => {}} onDelete={() => {}} />,
    );
    const [ownDelete, otherDelete] = r.getAllByRole('button', { name: 'Kullanıcıyı sil' });
    expect(ownDelete).toBeDisabled();
    expect(otherDelete).toBeEnabled();
  });

  it('hides every control from a read-only role', () => {
    const r = renderWithProviders(
      <UsersTabView users={users} selfId="u-1" canEdit={false} onAdd={() => {}} onEdit={() => {}} onDelete={() => {}} />,
    );
    expect(r.queryByRole('button', { name: 'Kullanıcı ekle' })).toBeNull();
    expect(r.queryByRole('button', { name: 'Kullanıcıyı sil' })).toBeNull();
  });

  it('edits a user', async () => {
    const onEdit = vi.fn();
    const r = renderWithProviders(
      <UsersTabView users={users} selfId="u-1" canEdit onAdd={() => {}} onEdit={onEdit} onDelete={() => {}} />,
    );
    await r.user.click(r.getAllByRole('button', { name: 'Kullanıcıyı düzenle' })[1]);
    expect(onEdit).toHaveBeenCalledWith(users[1]);
  });
});
