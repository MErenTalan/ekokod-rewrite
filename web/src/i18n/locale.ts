export const locales = ['tr', 'en'] as const;
export type Locale = (typeof locales)[number];
export const defaultLocale: Locale = 'tr';
/** Cookie-based locale, no URL prefix for the product (plan D8). */
export const LOCALE_COOKIE = 'NEXT_LOCALE';

export function resolveLocale(value?: string): Locale {
  return (locales as readonly string[]).includes(value ?? '') ? (value as Locale) : defaultLocale;
}
