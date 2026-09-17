'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useState, type FormEvent } from 'react';

import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { FormErrorSummary } from '@/components/ui/form-error-summary';
import { Input } from '@/components/ui/input';
import { api } from '@/lib/api/client';
import { errorCode, safeNext } from '@/lib/api/errors';

import { adoptProfilePreferences } from './adopt-preferences';
import { AuthFeedback } from './_auth-feedback';
import { authLinkClass } from './auth-link';
import { retryMinutes } from './retry-minutes';

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export type LoginFormProps = { next?: string; reason?: string };

export function LoginForm({ next, reason }: LoginFormProps) {
  const t = useTranslations('auth');
  const forms = useTranslations('forms');
  const router = useRouter();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [remember, setRemember] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<{ email?: string; password?: string }>({});
  const [failure, setFailure] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const errors = {
      email: !email.trim() ? t('errors.emailRequired') : EMAIL.test(email.trim()) ? undefined : t('errors.emailInvalid'),
      password: password ? undefined : t('errors.passwordRequired'),
    };
    setFieldErrors(errors);
    setFailure(null);
    if (errors.email || errors.password) return;
    setPending(true);
    try {
      const { data, error, response } = await api.POST('/api/v1/auth/login', {
        body: { email: email.trim(), password, remember_me: remember },
      });
      if (data) {
        await adoptProfilePreferences(data);
        router.replace(safeNext(next));
        return;
      }
      const code = errorCode(error);
      if (code === 'invalid_credentials') setFailure(t('errors.invalidCredentials'));
      else if (response.status === 429) setFailure(t('errors.rateLimited', { minutes: retryMinutes(response) }));
      else setFailure(t('errors.generic'));
    } catch {
      setFailure(t('errors.generic'));
    }
    setPending(false);
  }

  const summary = [
    fieldErrors.email ? { fieldId: 'login-email', message: fieldErrors.email } : null,
    fieldErrors.password ? { fieldId: 'login-password', message: fieldErrors.password } : null,
  ].filter((e) => e !== null);

  return (
    <form noValidate onSubmit={submit} className="flex flex-col gap-5">
      <div className="flex flex-col gap-1">
        <h1 className="text-foreground type-h1">{t('login.title')}</h1>
        <p className="text-foreground-muted type-body">{t('login.description')}</p>
      </div>
      {reason === 'device_mismatch' ? <Alert tone="warning" title={t('reasons.deviceMismatch')} /> : null}
      {reason === 'password_reset' ? <Alert tone="success" title={t('reasons.passwordReset')} /> : null}
      {failure ? <AuthFeedback tone="danger" message={failure} /> : null}
      <FormErrorSummary title={forms('errorSummaryTitle')} errors={summary} />
      <Input
        id="login-email"
        label={t('login.email')}
        type="email"
        autoComplete="username"
        inputMode="email"
        value={email}
        error={fieldErrors.email}
        onChange={(e) => setEmail(e.target.value)}
      />
      <Input
        id="login-password"
        label={t('login.password')}
        type="password"
        autoComplete="current-password"
        value={password}
        error={fieldErrors.password}
        onChange={(e) => setPassword(e.target.value)}
      />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Checkbox label={t('login.remember')} checked={remember} onCheckedChange={setRemember} />
        <Link
          href="/auth/forgot-password"
          className={authLinkClass}
        >
          {t('login.forgot')}
        </Link>
      </div>
      <Button type="submit" size="lg" loading={pending} className="w-full">
        {t('login.submit')}
      </Button>
    </form>
  );
}
