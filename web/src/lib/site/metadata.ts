import type { Metadata } from 'next';
import { getLocale, getTranslations } from 'next-intl/server';

import { siteUrl } from './seo';

export type MetaKey = 'home' | 'about' | 'references' | 'documents' | 'toolkit' | 'pricing' | 'requestDemo' | 'contact' | 'blog' | 'calculator';

/** Title, description, canonical URL and Open Graph for one public page. */
export async function pageMetadata(key: MetaKey, path: string, extra: { image?: string; type?: 'website' | 'article'; title?: string; description?: string } = {}): Promise<Metadata> {
  const [t, locale] = await Promise.all([getTranslations('site.meta'), getLocale()]);
  const title = extra.title ?? t(`${key}.title`);
  const description = extra.description ?? t(`${key}.description`);
  return {
    metadataBase: new URL(siteUrl()),
    title: key === 'home' ? { absolute: title } : title,
    description,
    alternates: { canonical: path },
    openGraph: {
      title,
      description,
      url: path,
      siteName: t('siteName'),
      locale: locale === 'tr' ? 'tr_TR' : 'en_GB',
      type: extra.type ?? 'website',
      ...(extra.image ? { images: [{ url: extra.image }] } : {}),
    },
    twitter: { card: extra.image ? 'summary_large_image' : 'summary', title, description },
  };
}
