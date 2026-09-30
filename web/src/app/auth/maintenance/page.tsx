import type { Metadata } from 'next';
import { getTranslations } from 'next-intl/server';

import { AuthMessage } from '@/features/auth/auth-message';
import { CHOOSER_PATH } from '@/lib/api/errors';

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations('auth.maintenance');
  return { title: `${t('title')} · ekokod` };
}

export default async function MaintenancePage() {
  const t = await getTranslations('auth.maintenance');
  return <AuthMessage tone="warning" title={t('title')} description={t('description')} action={{ href: CHOOSER_PATH, label: t('retry') }} />;
}
