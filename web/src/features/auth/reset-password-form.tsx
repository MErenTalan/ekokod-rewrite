'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useState, type FormEvent } from 'react';

import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { FormErrorSummary } from '@/components/ui/form-error-summary';
import { Input } from '@/components/ui/input';
import { api } from '@/lib/api/client';
import { errorCode } from '@/lib/api/errors';

import { AuthFeedback } from './_auth-feedback';
import { authLinkClass } from './auth-link';
import { passwordChecks, violationKey } from './password-policy';
import { PasswordRules } from './password-rules';
import { retryMinutes } from './retry-minutes';


export function ResetPasswordForm({ token }: { token?: string }) {
  const t = useTranslations('auth');
  const forms = useTranslations('forms');
  const router = useRouter();
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [errors, setErrors] = useState<{ password?: string; confirm?: string }>({});
  const [failure, setFailure] = useState<{ message: string; tokenInvalid?: boolean } | null>(null);
  const [pending, setPending] = useState(false);

  if (!token) {
    return (
      <div className="flex flex-col gap-5">
        <h1 className="text-foreground type-h1">{t('reset.title')}</h1>
        <Alert tone="danger" title={t('reset.missingToken')} />
        <Link href="/auth/forgot-password" className={authLinkClass}>
          {t('reset.requestNew')}
        </Link>
      </div>
    );
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const next = {
      password: passwordChecks(password).every((c) => c.met) ? undefined : t('violations.passwordComplexity'),
      confirm: confirm === password ? undefined : t('errors.passwordMismatch'),
    };
    if (!password) next.password = t('errors.passwordRequired');
    else if ([...password].length < 10) next.password = t('violations.passwordTooShort');
    setErrors(next);
    setFailure(null);
    if (next.password || next.confirm) return;
    setPending(true);
    try {
      const { error, response } = await api.POST('/api/v1/auth/reset-password', { body: { token: token!, password } });
      if (response.ok) {
        router.replace('/auth/login?reason=password_reset');
        return;
      }
      const code = errorCode(error);
      const details = (error as { error?: { details?: Record<string, unknown> } } | undefined)?.error?.details;
      const codes = Array.isArray(details?.password) ? (details.password as string[]) : [];
      const messages = codes.map(violationKey).filter((k) => k !== null).map((k) => t(`violations.${k}`));
      if (code === 'reset_token_invalid') setFailure({ message: t('errors.resetTokenInvalid'), tokenInvalid: true });
      else if (messages.length) setErrors({ password: messages.join(' ') });
      else if (response.status === 429) setFailure({ message: t('errors.rateLimited', { minutes: retryMinutes(response) }) });
      else setFailure({ message: t('errors.generic') });
    } catch {
      setFailure({ message: t('errors.generic') });
    }
    setPending(false);
  }

  const summary = [
    errors.password ? { fieldId: 'reset-password', message: errors.password } : null,
    errors.confirm ? { fieldId: 'reset-confirm', message: errors.confirm } : null,
  ].filter((e) => e !== null);

  return (
    <form noValidate onSubmit={submit} className="flex flex-col gap-5">
      <div className="flex flex-col gap-1">
        <h1 className="text-foreground type-h1">{t('reset.title')}</h1>
        <p className="text-foreground-muted type-body">{t('reset.description')}</p>
      </div>
      {failure ? <AuthFeedback tone="danger" message={failure.message} /> : null}
      {failure?.tokenInvalid ? (
        <Link href="/auth/forgot-password" className={authLinkClass}>
          {t('reset.requestNew')}
        </Link>
      ) : null}
      <FormErrorSummary title={forms('errorSummaryTitle')} errors={summary} />
      <Input
        id="reset-password"
        label={t('reset.password')}
        type="password"
        autoComplete="new-password"
        value={password}
        error={errors.password}
        aria-describedby={errors.password ? 'reset-password-error reset-rules' : 'reset-rules'}
        onChange={(e) => setPassword(e.target.value)}
      />
      <PasswordRules id="reset-rules" password={password} />
      <Input
        id="reset-confirm"
        label={t('reset.confirm')}
        type="password"
        autoComplete="new-password"
        value={confirm}
        error={errors.confirm}
        onChange={(e) => setConfirm(e.target.value)}
      />
      <Button type="submit" size="lg" loading={pending} className="w-full">
        {t('reset.submit')}
      </Button>
    </form>
  );
}
