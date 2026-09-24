import { render, type RenderResult } from '@testing-library/react';
import userEvent, { type UserEvent } from '@testing-library/user-event';
import { NextIntlClientProvider } from 'next-intl';
import type { ReactElement } from 'react';

import { AppProviders } from '@/components/providers';
import type { Locale } from '@/i18n/locale';
import { defaultUiPreferences, type UiPreferences } from '@/lib/ui-preferences';
import { UiPreferencesProvider } from '@/lib/ui-preferences-provider';

import { messages } from '../../messages';

export function renderWithProviders(
  ui: ReactElement,
  o: { locale?: Locale; preferences?: Partial<UiPreferences> } = {},
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
        <UiPreferencesProvider initial={{ ...defaultUiPreferences, ...o.preferences }}>
          <AppProviders>{children}</AppProviders>
        </UiPreferencesProvider>
      </NextIntlClientProvider>
    ),
  });
  return { ...result, user };
}
