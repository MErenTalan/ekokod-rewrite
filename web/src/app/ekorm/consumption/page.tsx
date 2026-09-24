import { getTranslations } from 'next-intl/server';

import { ConsumptionPage } from '@/features/consumption/consumption-page';

export const dynamic = 'force-dynamic';

export async function generateMetadata() {
  const t = await getTranslations('consumption');
  return { title: t('title') };
}

export default function Page() {
  return <ConsumptionPage />;
}
