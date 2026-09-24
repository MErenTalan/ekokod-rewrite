import { getTranslations } from 'next-intl/server';

import { PredictPage } from '@/features/forecast/predict-page';

export const dynamic = 'force-dynamic';

export async function generateMetadata() {
  const t = await getTranslations('forecast.predict');
  return { title: t('title') };
}

export default function Page() {
  return <PredictPage />;
}
