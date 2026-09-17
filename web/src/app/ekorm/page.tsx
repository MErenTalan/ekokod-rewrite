import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/components/shell/page-header';
import { fetchHealth } from '@/lib/api';

import { HealthStatus } from '../_components/health-status';

export const dynamic = 'force-dynamic';

export default async function HomePage() {
  const [t, report] = await Promise.all([getTranslations('health'), fetchHealth()]);

  return (
    <>
      <PageHeader title={t('title')} description={t('subtitle')} />
      <HealthStatus report={report} />
    </>
  );
}
