'use client';

import { Check, Languages } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { useLocale, useTranslations } from 'next-intl';
import { useTransition } from 'react';

import { setLocale } from '@/i18n/actions';
import { locales } from '@/i18n/locale';

import { DropdownMenu } from '../ui/dropdown-menu';
import { IconButton } from '../ui/icon-button';

export function LanguageSwitcher() {
  const t = useTranslations('shell');
  const current = useLocale();
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  return (
    <DropdownMenu
      trigger={<IconButton label={t('language')} icon={Languages} aria-busy={pending || undefined} />}
      items={locales.map((locale) => ({
        type: 'item' as const,
        label: t(`languages.${locale}`),
        icon: locale === current ? Check : undefined,
        onSelect: () =>
          startTransition(async () => {
            await setLocale(locale);
            router.refresh();
          }),
      }))}
    />
  );
}
