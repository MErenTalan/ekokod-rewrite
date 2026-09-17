import type { Metadata } from 'next';
import { getTranslations } from 'next-intl/server';

import { AuthMessage } from '@/features/auth/auth-message';

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations('auth.error');
  return { title: `${t('title')} · EKORM` };
}

export default async function AuthErrorPage() {
  const t = await getTranslations('auth.error');
  return <AuthMessage tone="danger" title={t('title')} description={t('description')} action={{ href: '/auth/login', label: t('back') }} />;
}
