import { cookies } from 'next/headers';
import { getRequestConfig } from 'next-intl/server';

import { messages } from '../../messages';
import { LOCALE_COOKIE, resolveLocale } from './locale';

export default getRequestConfig(async () => {
  const locale = resolveLocale((await cookies()).get(LOCALE_COOKIE)?.value);
  return { locale, messages: messages[locale], timeZone: 'Europe/Istanbul' };
});
