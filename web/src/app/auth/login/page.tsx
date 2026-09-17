import type { Metadata } from 'next';
import { getTranslations } from 'next-intl/server';

import { LoginForm } from '@/features/auth/login-form';

import { firstParam, type SearchParams } from '../search-params';

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations('auth.login');
  return { title: `${t('title')} · EKORM` };
}

export default async function LoginPage({ searchParams }: { searchParams: SearchParams }) {
  const params = await searchParams;
  return <LoginForm next={firstParam(params.next)} reason={firstParam(params.reason)} />;
}
