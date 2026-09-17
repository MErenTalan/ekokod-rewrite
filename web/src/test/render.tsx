import { render, type RenderResult } from '@testing-library/react';
import userEvent, { type UserEvent } from '@testing-library/user-event';
import { NextIntlClientProvider } from 'next-intl';
import type { ReactElement } from 'react';

import { AppProviders } from '@/components/providers';
import type { Locale } from '@/i18n/locale';

import { messages } from '../../messages';

export function renderWithProviders(
  ui: ReactElement,
  o: { locale?: Locale } = {},
): RenderResult & { user: UserEvent } {
  const locale = o.locale ?? 'tr';
  const user = userEvent.setup();
  const result = render(ui, {
    wrapper: ({ children }) => (
      <NextIntlClientProvider
        locale={locale}
        messages={messages[locale]}
        timeZone="Europe/Istanbul"
        onError={(error) => {
          throw error;
        }}
      >
        <AppProviders>{children}</AppProviders>
      </NextIntlClientProvider>
    ),
  });
  return { ...result, user };
}
