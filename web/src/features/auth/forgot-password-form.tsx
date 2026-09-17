'use client';

import Link from 'next/link';
import { useTranslations } from 'next-intl';
import { useState, type FormEvent } from 'react';

import { Button } from '@/components/ui/button';
import { FormErrorSummary } from '@/components/ui/form-error-summary';
import { Input } from '@/components/ui/input';
import { api } from '@/lib/api/client';

import { AuthFeedback } from './_auth-feedback';
import { authLinkClass } from './auth-link';
import { retryMinutes } from './retry-minutes';

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Always the same confirmation, whether or not the address has an account (no enumeration). */
export function ForgotPasswordForm() {
  const t = useTranslations('auth');
  const forms = useTranslations('forms');
  const [email, setEmail] = useState('');
  const [error, setError] = useState<string>();
  const [failure, setFailure] = useState<string | null>(null);
  const [sent, setSent] = useState(false);
  const [pending, setPending] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const invalid = !email.trim() ? t('errors.emailRequired') : EMAIL.test(email.trim()) ? undefined : t('errors.emailInvalid');
    setError(invalid);
    setFailure(null);
    if (invalid) return;
    setPending(true);
    try {
      const { response } = await api.POST('/api/v1/auth/forgot-password', { body: { email: email.trim() } });
      if (response.status === 429) setFailure(t('errors.rateLimited', { minutes: retryMinutes(response) }));
      else setSent(true);
    } catch {
      setFailure(t('errors.generic'));
    }
    setPending(false);
  }

  const back = (
    <Link href="/auth/login" className={authLinkClass}>
      {t('forgot.back')}
    </Link>
  );

  if (sent) {
    return (
      <div className="flex flex-col gap-5">
        <h1 className="text-foreground type-h1">{t('forgot.sentTitle')}</h1>
        <AuthFeedback tone="success" message={t('forgot.sent')} />
        {back}
      </div>
    );
  }

  return (
    <form noValidate onSubmit={submit} className="flex flex-col gap-5">
      <div className="flex flex-col gap-1">
        <h1 className="text-foreground type-h1">{t('forgot.title')}</h1>
        <p className="text-foreground-muted type-body">{t('forgot.description')}</p>
      </div>
      {failure ? <AuthFeedback tone="danger" message={failure} /> : null}
      <FormErrorSummary title={forms('errorSummaryTitle')} errors={error ? [{ fieldId: 'forgot-email', message: error }] : []} />
      <Input
        id="forgot-email"
        label={t('login.email')}
        type="email"
        autoComplete="username"
        inputMode="email"
        value={email}
        error={error}
        onChange={(e) => setEmail(e.target.value)}
      />
      <Button type="submit" size="lg" loading={pending} className="w-full">
        {t('forgot.submit')}
      </Button>
      {back}
    </form>
  );
}
