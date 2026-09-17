import type { Metadata } from 'next';
import { getTranslations } from 'next-intl/server';

import { ResetPasswordForm } from '@/features/auth/reset-password-form';

import { firstParam, type SearchParams } from '../search-params';

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations('auth.reset');
  // The token is in the URL: never leak it through the Referer of anything this page loads.
  return { title: `${t('title')} · EKORM`, referrer: 'no-referrer' };
}

export default async function ResetPasswordPage({ searchParams }: { searchParams: SearchParams }) {
  return <ResetPasswordForm token={firstParam((await searchParams).token)} />;
}
