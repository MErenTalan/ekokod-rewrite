import { getTranslations } from 'next-intl/server';

import { LoadProfilePage } from '@/features/load-profile/load-profile-page';

export const dynamic = 'force-dynamic';

export async function generateMetadata() {
  const t = await getTranslations('loadProfile');
  return { title: t('title') };
}

export default function Page() {
  return <LoadProfilePage />;
}
