import { NextIntlClientProvider } from 'next-intl';
import { getLocale, getMessages } from 'next-intl/server';
import type { Metadata } from 'next';
import { cookies } from 'next/headers';

import { AppProviders } from '@/components/providers';
import { htmlAttributes, parseUiPreferences, UI_COOKIE } from '@/lib/ui-preferences';
import { UiPreferencesProvider } from '@/lib/ui-preferences-provider';
import { fontVariables } from '@/styles/fonts';

import './globals.css';

export const metadata: Metadata = {
  title: 'ekokod',
  description: 'Enerji yönetim platformu',
};

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const [locale, messages, cookieStore] = await Promise.all([getLocale(), getMessages(), cookies()]);
  const prefs = parseUiPreferences(cookieStore.get(UI_COOKIE)?.value);

  // Preferences are rendered as <html data-*> on the server: no theme or layout flash, no inline script (plan D4, D15).
  return (
    <html lang={locale} className={fontVariables} {...htmlAttributes(prefs)}>
      <body>
        <NextIntlClientProvider messages={messages}>
          <UiPreferencesProvider initial={prefs}>
            <AppProviders>{children}</AppProviders>
          </UiPreferencesProvider>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
