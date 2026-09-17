'use client';

import { useTranslations } from 'next-intl';

import { PasswordRules } from '@/features/auth/password-rules';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import type { Role, UserCreateRequest } from '@/lib/api/types';

/** Role labels live in the shell namespace, where the user menu already needs them. */
export const ROLE_LABEL_KEY = {
  admin: 'admin',
  company_admin: 'companyAdmin',
  company_readonly_admin: 'companyReadonlyAdmin',
  building_admin: 'buildingAdmin',
  building_readonly_admin: 'buildingReadonlyAdmin',
  demo: 'demo',
} as const satisfies Record<Role, string>;

/**
 * The roles a caller may hand out (R174): an admin every real role, a company
 * admin everything but admin; demo users exist only through `seed demo`.
 */
export function assignableRoles(actor: Role): Role[] {
  const all: Role[] = ['admin', 'company_admin', 'company_readonly_admin', 'building_admin', 'building_readonly_admin'];
  return actor === 'admin' ? all : all.filter((role) => role !== 'admin');
}

export type UserDraft = {
  id?: string;
  name: string;
  email: string;
  phone: string;
  role: Role;
  password: string;
  isActive: boolean;
};

export const emptyUser = (actor: Role): UserDraft => ({
  name: '',
  email: '',
  phone: '',
  role: assignableRoles(actor)[assignableRoles(actor).length - 1],
  password: '',
  isActive: true,
});

export function toUserRequest(draft: UserDraft): UserCreateRequest {
  return {
    name: draft.name,
    email: draft.email,
    phone: draft.phone || null,
    role: draft.role,
    is_active: draft.isActive,
    password: draft.password,
  };
}

export type UserFormViewProps = {
  value: UserDraft;
  onChange: (draft: UserDraft) => void;
  actorRole: Role;
  /** The caller's own row: role and active state are the API's to refuse (R174). */
  isSelf?: boolean;
  fieldErrors?: Record<string, string>;
};

/** User fields of 01 §7.15, with the role options the caller may assign. */
export function UserFormView({ value, onChange, actorRole, isSelf = false, fieldErrors = {} }: UserFormViewProps) {
  const t = useTranslations('settings.users');
  const roles = useTranslations('shell.roles');
  const set = (patch: Partial<UserDraft>) => onChange({ ...value, ...patch });

  return (
    <div className="flex flex-col gap-4">
      <Input label={t('name')} value={value.name} error={fieldErrors.name} onChange={(e) => set({ name: e.target.value })} required />
      <Input label={t('email')} type="email" value={value.email} error={fieldErrors.email} onChange={(e) => set({ email: e.target.value })} required />
      <Input label={t('phone')} value={value.phone} error={fieldErrors.phone} onChange={(e) => set({ phone: e.target.value })} />
      <Select
        label={t('role')}
        options={assignableRoles(actorRole).map((role) => ({ value: role, label: roles(ROLE_LABEL_KEY[role]) }))}
        value={value.role}
        error={fieldErrors.role}
        disabled={isSelf}
        description={isSelf ? t('cannotModifySelf') : undefined}
        onValueChange={(role) => set({ role: role as Role })}
      />
      <Switch label={t('active')} checked={value.isActive} disabled={isSelf} onCheckedChange={(isActive) => set({ isActive })} />
      {value.id ? null : (
        <>
          <Input
            label={t('password')}
            type="password"
            autoComplete="new-password"
            value={value.password}
            error={fieldErrors.password}
            onChange={(e) => set({ password: e.target.value })}
            required
          />
          <PasswordRules password={value.password} />
        </>
      )}
    </div>
  );
}
