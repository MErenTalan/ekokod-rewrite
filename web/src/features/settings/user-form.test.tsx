import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { UserFormView, assignableRoles, emptyUser, toUserRequest } from './user-form';

describe('assignableRoles', () => {
  it('never lets a company admin create an admin (R174)', () => {
    expect(assignableRoles('company_admin')).toEqual([
      'company_admin',
      'company_readonly_admin',
      'building_admin',
      'building_readonly_admin',
    ]);
    expect(assignableRoles('admin')).toContain('admin');
  });

  it('never offers demo, which only the seed creates', () => {
    expect(assignableRoles('admin')).not.toContain('demo');
    expect(assignableRoles('company_admin')).not.toContain('demo');
  });
});

describe('UserFormView', () => {
  it('asks for a password only when creating', () => {
    const creating = renderWithProviders(
      <UserFormView value={emptyUser('company_admin')} onChange={() => {}} actorRole="company_admin" />,
    );
    expect(creating.getByLabelText(/Şifre/)).toBeInTheDocument();
    expect(creating.getByText(/En az 10 karakter/)).toBeInTheDocument();
    creating.unmount();

    const editing = renderWithProviders(
      <UserFormView
        value={{ ...emptyUser('company_admin'), id: 'u-2', name: 'Can', email: 'can@ornek.com.tr' }}
        onChange={() => {}}
        actorRole="company_admin"
      />,
    );
    expect(editing.queryByLabelText(/Şifre/)).toBeNull();
  });

  it('locks the role and active switch on the caller own row', () => {
    const r = renderWithProviders(
      <UserFormView
        value={{ ...emptyUser('admin'), id: 'u-1', name: 'Ben', email: 'ben@ornek.com.tr' }}
        onChange={() => {}}
        actorRole="admin"
        isSelf
      />,
    );
    expect(r.getByRole('combobox', { name: /Rol/ })).toBeDisabled();
    expect(r.getByRole('switch', { name: 'Aktif' })).toBeDisabled();
    expect(r.getByText('Kendi hesabınızı buradan değiştiremezsiniz')).toBeInTheDocument();
  });

  it('builds the create request', () => {
    expect(
      toUserRequest({ ...emptyUser('company_admin'), name: 'Yeni', email: 'yeni@ornek.com.tr', password: 'Guvenli!Sifre-42' }),
    ).toEqual({
      name: 'Yeni',
      email: 'yeni@ornek.com.tr',
      phone: null,
      role: 'building_readonly_admin',
      is_active: true,
      password: 'Guvenli!Sifre-42',
    });
  });
});
