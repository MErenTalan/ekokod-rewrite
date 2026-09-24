'use client';

import { Activity, Leaf, ReceiptText } from 'lucide-react';
import { useTranslations } from 'next-intl';
import type { ReactNode } from 'react';

import { LanguageSwitcher } from '@/components/shell/language-switcher';
import { ThemeToggle } from '@/components/shell/theme-toggle';

/** 01 §7.1 split layout: the form, and a brand panel that only appears from 768 px. */
export function AuthSplitLayout({ children }: { children: ReactNode }) {
  const t = useTranslations('auth.brand');
  const app = useTranslations('app');
  const points = [
    { key: 'consumption', Icon: Activity },
    { key: 'bills', Icon: ReceiptText },
    { key: 'carbon', Icon: Leaf },
  ] as const;
  return (
    <div className="grid min-h-dvh bg-background md:grid-cols-2">
      <main className="flex min-w-0 flex-col px-4 py-4 sm:px-8">
        <header className="flex items-center justify-between gap-2">
          <span className="flex items-center gap-1.5">
            <Leaf aria-hidden className="size-6 stroke-brand" />
            <span className="font-heading text-lg font-semibold text-foreground">{app('name')}</span>
          </span>
          <span className="flex items-center gap-1">
            <LanguageSwitcher />
            <ThemeToggle />
          </span>
        </header>
        <div className="mx-auto flex w-full max-w-sm flex-1 flex-col justify-center py-10">{children}</div>
      </main>
      <aside className="hidden flex-col justify-center gap-8 border-s border-border bg-surface-sunken p-12 md:flex">
        <div className="flex max-w-md flex-col gap-3">
          <Leaf aria-hidden className="size-10 stroke-brand" />
          <p className="text-foreground type-display">{t('title')}</p>
          <p className="text-foreground-muted type-body-lg">{t('description')}</p>
        </div>
        <ul className="flex max-w-md flex-col gap-4">
          {points.map(({ key, Icon }) => (
            <li key={key} className="flex items-start gap-3 text-foreground type-body-lg">
              <Icon aria-hidden className="mt-0.5 size-5 shrink-0 stroke-brand" />
              <span>{t(`points.${key}`)}</span>
            </li>
          ))}
        </ul>
      </aside>
    </div>
  );
}
