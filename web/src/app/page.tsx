import { getTranslations } from 'next-intl/server';

import { HealthStatus } from './_components/health-status';
import { fetchHealth } from '@/lib/api';

export const dynamic = 'force-dynamic';

export default async function HomePage() {
  const [t, report] = await Promise.all([getTranslations('health'), fetchHealth()]);

  return (
    <main className="mx-auto max-w-2xl p-8">
      <h1 className="type-h1">{t('title')}</h1>
      <p className="mt-1 mb-6 text-foreground-muted">{t('subtitle')}</p>
      <HealthStatus report={report} />
    </main>
  );
}
