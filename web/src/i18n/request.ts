import { getRequestConfig } from 'next-intl/server';

export const locales = ['tr', 'en'] as const;
export const defaultLocale = 'tr';

export default getRequestConfig(async () => {
  const locale = defaultLocale;
  return { locale, messages: (await import(`../../messages/${locale}.json`)).default };
});
