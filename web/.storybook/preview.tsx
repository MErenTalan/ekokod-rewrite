import type { Decorator, Preview } from '@storybook/nextjs-vite';
import { NextIntlClientProvider } from 'next-intl';
import '../src/app/globals.css';
import { messages } from '../messages';
import { AppProviders } from '../src/components/providers';
import { defaultUiPreferences, htmlAttributes, type UiPreferences } from '../src/lib/ui-preferences';
import { UiPreferencesProvider } from '../src/lib/ui-preferences-provider';
import { fontVariables } from '../src/styles/fonts';

const withAppContext: Decorator = (Story, { globals, parameters }) => {
  const theme = globals.theme === 'dark' ? 'dark' : 'light';
  const locale = globals.locale === 'en' ? 'en' : 'tr';
  const prefs: UiPreferences = { ...defaultUiPreferences, ...(parameters.preferences as Partial<UiPreferences>), theme };
  const html = document.documentElement; // synchronous: axe must never see a stale theme
  const { style, ...data } = htmlAttributes(prefs);
  for (const [name, value] of Object.entries(data)) html.setAttribute(name, value);
  html.style.setProperty('--radius-scale', String((style as Record<string, string>)['--radius-scale']));
  html.lang = locale; html.classList.add(...fontVariables.split(' '));
  return (
    <NextIntlClientProvider locale={locale} messages={messages[locale]} timeZone="Europe/Istanbul"
      onError={(error) => { throw error; }}>
      {/* Theme comes from the toolbar; stories may set other preferences (layout, sidebar…) via parameters.preferences. */}
      <UiPreferencesProvider key={JSON.stringify(prefs)} initial={prefs}>
        <AppProviders><Story /></AppProviders>
      </UiPreferencesProvider>
    </NextIntlClientProvider>
  );
};

const preview: Preview = {
  decorators: [withAppContext],
  initialGlobals: { theme: 'light', locale: 'tr' },
  globalTypes: { theme: { toolbar: { title: 'Theme', items: ['light', 'dark'] } },
    locale: { toolbar: { title: 'Locale', items: ['tr', 'en'] } } },
  parameters: { layout: 'padded', nextjs: { appDirectory: true }, a11y: { test: 'error' } },
};
export default preview;
