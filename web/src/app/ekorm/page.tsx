import { getTranslations } from 'next-intl/server';

import { DashboardPage } from '@/features/dashboard/dashboard-page';

export const dynamic = 'force-dynamic';

export async function generateMetadata() {
  const t = await getTranslations('dashboard');
  return { title: t('title') };
}

export default function Page() {
  return <DashboardPage />;
}
