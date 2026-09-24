'use client';

import { Check, X } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { cn } from '@/lib/cn';

import { passwordChecks } from './password-policy';

/** Live rule list under a new-password field; each rule is icon + text + an announced state (07 §2.4). */
export function PasswordRules({ password, id }: { password: string; id?: string }) {
  const t = useTranslations('auth.policy');
  return (
    <div id={id} className="flex flex-col gap-1.5">
      <p className="text-foreground type-small font-semibold">{t('title')}</p>
      <ul className="flex flex-col gap-1">
        {passwordChecks(password).map(({ rule, met }) => (
          <li key={rule} className={cn('flex items-start gap-1.5 type-small', met ? 'text-foreground' : 'text-foreground-muted')}>
            {met ? (
              <Check aria-hidden className="mt-0.5 size-3.5 shrink-0 stroke-success" />
            ) : (
              <X aria-hidden className="mt-0.5 size-3.5 shrink-0" />
            )}
            <span>
              {t(rule)}
              <span className="sr-only">: {met ? t('met') : t('unmet')}</span>
            </span>
          </li>
        ))}
        <li className="flex items-start gap-1.5 text-foreground-muted type-small">
          <span aria-hidden className="mt-0.5 size-3.5 shrink-0" />
          <span>{t('personal')}</span>
        </li>
      </ul>
    </div>
  );
}
