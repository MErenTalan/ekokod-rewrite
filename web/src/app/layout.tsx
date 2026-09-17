import { NextIntlClientProvider } from 'next-intl';
import { getLocale, getMessages } from 'next-intl/server';
import type { Metadata } from 'next';

import { fontVariables } from '@/styles/fonts';

import './globals.css';

export const metadata: Metadata = {
  title: 'ekokod',
  description: 'Enerji yönetim platformu',
};

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const locale = await getLocale();
  const messages = await getMessages();

  // data-theme="system" + color-scheme picks light or dark from the OS with no script (plan D4).
  return (
    <html lang={locale} data-theme="system" className={fontVariables}>
      <body>
        <NextIntlClientProvider messages={messages}>{children}</NextIntlClientProvider>
      </body>
    </html>
  );
}
