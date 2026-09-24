import { getTranslations } from 'next-intl/server';

import { pageMetadata } from '@/lib/site/metadata';

export const generateMetadata = () => pageMetadata('home', '/');

export default async function HomePage() {
  const t = await getTranslations('site.home.hero');
  return <h1 className="mx-auto max-w-7xl px-4 py-16 type-display">{t('title')}</h1>;
}
