import { getTranslations } from 'next-intl/server';

import { AiPage } from '@/features/forecast/ai-page';

export const dynamic = 'force-dynamic';

export async function generateMetadata() {
  const t = await getTranslations('forecast.ai');
  return { title: t('title') };
}

export default function Page() {
  return <AiPage />;
}
