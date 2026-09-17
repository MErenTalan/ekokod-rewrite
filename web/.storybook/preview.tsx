import type { Decorator, Preview } from '@storybook/nextjs-vite';
import { NextIntlClientProvider } from 'next-intl';
import '../src/app/globals.css';
import { messages } from '../messages';
import { AppProviders } from '../src/components/providers';
import { fontVariables } from '../src/styles/fonts';

const withAppContext: Decorator = (Story, { globals }) => {
  const theme = globals.theme === 'dark' ? 'dark' : 'light';
  const locale = globals.locale === 'en' ? 'en' : 'tr';
  const html = document.documentElement; // synchronous: axe must never see a stale theme
  html.dataset.theme = theme; html.lang = locale; html.classList.add(...fontVariables.split(' '));
  return (
    <NextIntlClientProvider locale={locale} messages={messages[locale]} timeZone="Europe/Istanbul"
      onError={(error) => { throw error; }}>
      <AppProviders><Story /></AppProviders>
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
