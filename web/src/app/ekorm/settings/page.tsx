import { getTranslations } from 'next-intl/server';
import { Suspense } from 'react';

import { SettingsPage } from '@/features/settings/settings-page';

export const dynamic = 'force-dynamic';

export async function generateMetadata() {
  const t = await getTranslations('settings');
  return { title: t('title') };
}

export default function Page() {
  // useSearchParams needs a Suspense boundary during static prerendering.
  return (
    <Suspense>
      <SettingsPage />
    </Suspense>
  );
}
