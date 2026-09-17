import type { Messages } from '../../messages';
import type { Locale } from './locale';

declare module 'next-intl' {
  interface AppConfig {
    Locale: Locale;
    Messages: Messages;
  }
}
