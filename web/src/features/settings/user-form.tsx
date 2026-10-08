'use client';

import { useTranslations } from 'next-intl';

import { PasswordRules } from '@/features/auth/password-rules';
import { Input } from '@/components/ui/input';
import { MultiSelect } from '@/components/ui/multi-select';
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
  const all: Role[] = [
    'admin',
    'company_admin',
    'company_readonly_admin',
    'building_admin',
    'building_readonly_admin',
  ];
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
  /** The company a new user joins; an admin picks it (the API reads it as company_id). */
  companyId?: string;
  /** A building admin's buildings (buildings.responsible_user_id). */
  buildingIds: string[];
};

/** Building admins are scoped to the buildings they are responsible for (01 §3). */
export const isBuildingRole = (role: Role) =>
  role === 'building_admin' || role === 'building_readonly_admin';

export const emptyUser = (actor: Role, companyId?: string): UserDraft => ({
  name: '',
  email: '',
  phone: '',
  role: assignableRoles(actor)[assignableRoles(actor).length - 1],
  password: '',
  isActive: true,
  companyId,
  buildingIds: [],
});

export function toUserRequest(draft: UserDraft): UserCreateRequest {
  return {
    name: draft.name,
    email: draft.email,
    phone: draft.phone || null,
    role: draft.role,
    is_active: draft.isActive,
    password: draft.password,
    ...(isBuildingRole(draft.role) ? { building_ids: draft.buildingIds } : {}),
  };
}

export type NamedOption = { id: string; name: string };

export type UserFormViewProps = {
  value: UserDraft;
  onChange: (draft: UserDraft) => void;
  actorRole: Role;
  /** The caller's own row: role and active state are the API's to refuse (R174). */
  isSelf?: boolean;
  fieldErrors?: Record<string, string>;
  /** Companies an admin may create the user in; absent for everyone else. */
  companies?: NamedOption[];
  /** Buildings of the user's company, for a building admin's assignment. */
  buildings?: NamedOption[];
};

/** User fields of 01 §7.15, with the role options the caller may assign. */
export function UserFormView({
  value,
  onChange,
  actorRole,
  isSelf = false,
  fieldErrors = {},
  companies,
  buildings = [],
}: UserFormViewProps) {
  const t = useTranslations('settings.users');
  const roles = useTranslations('shell.roles');
  const set = (patch: Partial<UserDraft>) => onChange({ ...value, ...patch });

  return (
    <div className="flex flex-col gap-4">
      <Input
        label={t('name')}
        value={value.name}
        error={fieldErrors.name}
        onChange={(e) => set({ name: e.target.value })}
        required
      />
      <Input
        label={t('email')}
        type="email"
        value={value.email}
        error={fieldErrors.email}
        onChange={(e) => set({ email: e.target.value })}
        required
      />
      <Input
        label={t('phone')}
        value={value.phone}
        error={fieldErrors.phone}
        onChange={(e) => set({ phone: e.target.value })}
      />
      <Select
        label={t('role')}
        required
        options={assignableRoles(actorRole).map((role) => ({
          value: role,
          label: roles(ROLE_LABEL_KEY[role]),
        }))}
        value={value.role}
        error={fieldErrors.role}
        disabled={isSelf}
        description={isSelf ? t('cannotModifySelf') : undefined}
        onValueChange={(role) => set({ role: role as Role })}
      />
      {companies ? (
        <Select
          label={t('company')}
          error={fieldErrors.company_id}
          required
          options={companies.map((c) => ({ value: c.id, label: c.name }))}
          value={value.companyId ?? ''}
          disabled={Boolean(value.id)}
          description={t('companyHint')}
          onValueChange={(companyId) => set({ companyId, buildingIds: [] })}
        />
      ) : null}
      {isBuildingRole(value.role) ? (
        <MultiSelect
          label={t('buildings')}
          description={buildings.length ? t('buildingsHint') : t('noBuildings')}
          options={buildings.map((b) => ({ value: b.id, label: b.name }))}
          value={value.buildingIds}
          error={fieldErrors.building_ids}
          searchPlaceholder={t('buildingsSearch')}
          emptyText={t('noBuildings')}
          onValueChange={(buildingIds) => set({ buildingIds })}
        />
      ) : null}
      <Switch
        label={t('active')}
        checked={value.isActive}
        disabled={isSelf}
        onCheckedChange={(isActive) => set({ isActive })}
      />
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
