'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { PasswordRules } from '@/features/auth/password-rules';
import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Table, TableBody, TableCaption, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Me, ProfileUpdateRequest, Session } from '@/lib/api/types';
import { formatDate } from '@/lib/format';
import type { Locale } from '@/i18n/locale';
import { useLocale } from 'next-intl';

export type AccountTabViewProps = {
  profile: Me;
  /** Demo is a shared account: it reads its profile but changes nothing (R150). */
  readOnly: boolean;
  sessions: Session[] | null;
  onSaveProfile: (values: ProfileUpdateRequest) => void;
  onChangePassword: (values: { current_password: string; new_password: string }) => void;
  onRevokeSession: (id: string) => void;
  onLogoutAll: () => void;
  fieldErrors?: Record<string, string>;
  saving?: 'profile' | 'password' | null;
};

/** The Account tab of 01 §7.15: personal details, password and active sessions. */
export function AccountTabView({
  profile,
  readOnly,
  sessions,
  onSaveProfile,
  onChangePassword,
  onRevokeSession,
  onLogoutAll,
  fieldErrors = {},
  saving = null,
}: AccountTabViewProps) {
  const t = useTranslations('settings.account');
  const common = useTranslations('common');
  const locale = useLocale() as Locale;
  const [values, setValues] = useState({ name: profile.name, email: profile.email, phone: profile.phone ?? '' });
  const [password, setPassword] = useState({ current: '', next: '' });

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader>
          <CardTitle>{t('profile')}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {readOnly ? <Alert tone="info" title={t('demoReadOnly')} /> : null}
          <form
            className="flex max-w-xl flex-col gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              onSaveProfile({ name: values.name, email: values.email, phone: values.phone || null });
            }}
          >
            <Input label={t('name')} value={values.name} disabled={readOnly} error={fieldErrors.name} onChange={(e) => setValues({ ...values, name: e.target.value })} required />
            <Input label={t('email')} type="email" value={values.email} disabled={readOnly} error={fieldErrors.email} onChange={(e) => setValues({ ...values, email: e.target.value })} required />
            <Input label={t('phone')} value={values.phone} disabled={readOnly} error={fieldErrors.phone} onChange={(e) => setValues({ ...values, phone: e.target.value })} />
            {!readOnly ? (
              <Button type="submit" loading={saving === 'profile'} className="self-start">
                {common('save')}
              </Button>
            ) : null}
          </form>
        </CardContent>
      </Card>

      {!readOnly ? (
        <Card>
          <CardHeader>
            <CardTitle>{t('password')}</CardTitle>
          </CardHeader>
          <CardContent>
            <form
              className="flex max-w-xl flex-col gap-4"
              onSubmit={(event) => {
                event.preventDefault();
                onChangePassword({ current_password: password.current, new_password: password.next });
              }}
            >
              <Input
                label={t('currentPassword')}
                type="password"
                autoComplete="current-password"
                value={password.current}
                error={fieldErrors.current_password}
                onChange={(e) => setPassword({ ...password, current: e.target.value })}
                required
              />
              <Input
                label={t('newPassword')}
                type="password"
                autoComplete="new-password"
                value={password.next}
                error={fieldErrors.new_password}
                onChange={(e) => setPassword({ ...password, next: e.target.value })}
                required
              />
              <PasswordRules password={password.next} />
              <Button type="submit" loading={saving === 'password'} className="self-start">
                {t('changePassword')}
              </Button>
            </form>
          </CardContent>
        </Card>
      ) : null}

      {!readOnly ? (
        <Card>
          <CardHeader className="flex flex-row items-start justify-between gap-2">
            <CardTitle>{t('sessions')}</CardTitle>
            <Button variant="secondary" size="sm" onClick={onLogoutAll}>
              {t('logoutAll')}
            </Button>
          </CardHeader>
          <CardContent>
            <TableContainer label={t('sessions')}>
              <Table>
                <TableCaption>{t('sessions')}</TableCaption>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('device')}</TableHead>
                    <TableHead>{t('ip')}</TableHead>
                    <TableHead>{t('lastUsed')}</TableHead>
                    <TableHead>{t('expires')}</TableHead>
                    <TableHead>
                      <span className="sr-only">{t('revoke')}</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(sessions ?? []).map((session) => (
                    <TableRow key={session.id}>
                      <TableCell>
                        {session.user_agent ?? t('unknownDevice')}
                        {session.current ? ` · ${t('thisDevice')}` : ''}
                      </TableCell>
                      <TableCell>{session.ip ?? '—'}</TableCell>
                      <TableCell>{session.last_used_at ? formatDate(session.last_used_at.slice(0, 10), locale) : '—'}</TableCell>
                      <TableCell>{formatDate(session.expires_at.slice(0, 10), locale)}</TableCell>
                      <TableCell>
                        {session.current ? null : (
                          <Button size="sm" variant="ghost" onClick={() => onRevokeSession(session.id)}>
                            {t('revoke')}
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}
